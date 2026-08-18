import { config } from './config.mjs';
import { pool, audit } from './db.mjs';
import { getShortUrl, editShortUrl, deleteShortUrl } from './shlink.mjs';

const DEFAULT_CONFIRM_AFTER_MS = 5 * 60_000;
const CHECK_INTERVAL_MINUTES = 60;

function normalizedUrl(value) {
  try { return new URL(String(value || '')).toString(); } catch { return null; }
}

export function trackedLinkMatches(link, upstream) {
  const expected = normalizedUrl(link?.long_url);
  const actual = normalizedUrl(upstream?.longUrl);
  return Boolean(expected && actual && expected === actual);
}

async function markMissing(link, reason) {
  const { rows } = await pool.query(
    `UPDATE links
     SET disabled_at=COALESCE(disabled_at,NOW()),consistency_checked_at=NOW(),consistency_mismatch_at=NULL,updated_at=NOW()
     WHERE id=$1 AND disabled_at IS NULL
     RETURNING id`,
    [link.id]
  );
  if (rows[0]) await audit(null, 'consistency.link_disabled', link.id, { shortCode: link.short_code, reason });
  return Boolean(rows[0]);
}

async function observeUpstream(link) {
  try {
    return { found: true, value: await getShortUrl(link.short_code) };
  } catch (error) {
    if (error?.status === 404) return { found: false, value: null };
    throw error;
  }
}

export async function reconcileTrackedLink(link, { confirmAfterMs = DEFAULT_CONFIRM_AFTER_MS } = {}) {
  let observed;
  try {
    observed = await observeUpstream(link);
  } catch (error) {
    console.warn(JSON.stringify({ level: 'warn', event: 'consistency.lookup_failed', linkId: link.id, message: error.message }));
    return 'lookup_failed';
  }

  if (!observed.found) {
    await markMissing(link, 'missing_upstream');
    return 'disabled_missing';
  }

  if (trackedLinkMatches(link, observed.value)) {
    await pool.query('UPDATE links SET consistency_checked_at=NOW(),consistency_mismatch_at=NULL WHERE id=$1 AND disabled_at IS NULL', [link.id]);
    return 'consistent';
  }

  const mismatchAt = link.consistency_mismatch_at ? new Date(link.consistency_mismatch_at).getTime() : 0;
  if (confirmAfterMs > 0 && (!mismatchAt || Date.now() - mismatchAt < confirmAfterMs)) {
    await pool.query('UPDATE links SET consistency_checked_at=NOW(),consistency_mismatch_at=COALESCE(consistency_mismatch_at,NOW()) WHERE id=$1 AND disabled_at IS NULL', [link.id]);
    if (!mismatchAt) await audit(null, 'consistency.mismatch_detected', link.id, { shortCode: link.short_code });
    return 'mismatch_pending';
  }

  // Re-read both sides after the confirmation window so a normal in-flight
  // QH8Z edit cannot be mistaken for persistent divergence.
  const currentResult = await pool.query(
    'SELECT id,short_code,long_url,title,updated_at,disabled_at,consistency_mismatch_at FROM links WHERE id=$1',
    [link.id]
  );
  const current = currentResult.rows[0];
  if (!current || current.disabled_at) return 'inactive';

  let confirmed;
  try {
    confirmed = await observeUpstream(current);
  } catch (error) {
    console.warn(JSON.stringify({ level: 'warn', event: 'consistency.confirm_lookup_failed', linkId: link.id, message: error.message }));
    return 'lookup_failed';
  }
  if (!confirmed.found) {
    await markMissing(current, 'missing_upstream_after_confirmation');
    return 'disabled_missing';
  }
  if (trackedLinkMatches(current, confirmed.value)) {
    await pool.query('UPDATE links SET consistency_checked_at=NOW(),consistency_mismatch_at=NULL WHERE id=$1', [current.id]);
    return 'consistent';
  }

  // The QH8Z row is the policy-checked ownership record. Repair Shlink back to
  // that state. If the mutation times out after committing, a follow-up GET
  // resolves the ambiguity before we consider fail-closed deletion.
  let repaired = false;
  try {
    await editShortUrl(current.short_code, { longUrl: current.long_url, title: current.title });
    repaired = true;
  } catch (error) {
    console.warn(JSON.stringify({ level: 'warn', event: 'consistency.repair_request_failed', linkId: current.id, message: error.message }));
  }

  try {
    const after = await observeUpstream(current);
    repaired = after.found && trackedLinkMatches(current, after.value);
  } catch (error) {
    console.warn(JSON.stringify({ level: 'warn', event: 'consistency.repair_verify_failed', linkId: current.id, message: error.message }));
  }

  if (repaired) {
    await pool.query('UPDATE links SET consistency_checked_at=NOW(),consistency_mismatch_at=NULL WHERE id=$1 AND disabled_at IS NULL', [current.id]);
    await audit(null, 'consistency.link_repaired', current.id, { shortCode: current.short_code });
    return 'repaired';
  }

  try {
    await deleteShortUrl(current.short_code);
  } catch (error) {
    if (error?.status !== 404) {
      console.error(JSON.stringify({ level: 'error', event: 'consistency.fail_closed_delete_failed', linkId: current.id, message: error.message }));
      return 'repair_failed';
    }
  }
  await markMissing(current, 'persistent_upstream_mismatch');
  return 'disabled_mismatch';
}

export async function reconcileDueLinks({ confirmAfterMs = DEFAULT_CONFIRM_AFTER_MS, batch = null } = {}) {
  const limit = Math.min(Math.max(Number(batch) || config.reputationRecheckBatch * 4 || 100, 25), 500);
  const { rows } = await pool.query(
    `SELECT id,short_code,long_url,title,updated_at,disabled_at,consistency_checked_at,consistency_mismatch_at
     FROM links
     WHERE disabled_at IS NULL
       AND (consistency_checked_at IS NULL
         OR consistency_checked_at < updated_at
         OR consistency_checked_at < NOW() - ($1::text || ' minutes')::interval)
     ORDER BY consistency_checked_at ASC NULLS FIRST, updated_at ASC
     LIMIT $2`,
    [CHECK_INTERVAL_MINUTES, limit]
  );

  const results = { checked: 0, consistent: 0, repaired: 0, disabled: 0, pending: 0, failed: 0 };
  for (const link of rows) {
    const outcome = await reconcileTrackedLink(link, { confirmAfterMs });
    results.checked += 1;
    if (outcome === 'consistent') results.consistent += 1;
    else if (outcome === 'repaired') results.repaired += 1;
    else if (outcome.startsWith('disabled_')) results.disabled += 1;
    else if (outcome === 'mismatch_pending') results.pending += 1;
    else if (outcome.endsWith('failed')) results.failed += 1;
  }
  return results;
}
