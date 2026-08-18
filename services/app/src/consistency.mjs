import { config } from './config.mjs';
import { pool, audit } from './db.mjs';
import { getShortUrl, editShortUrl, deleteShortUrl } from './shlink.mjs';

const DEFAULT_CONFIRM_AFTER_MS = 5 * 60_000;
const DEFAULT_ORPHAN_AFTER_MS = 5 * 60_000;
const CHECK_INTERVAL_MINUTES = 60;
const RECONCILE_CONCURRENCY = 10;

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

export async function reconcileCreateIntents({ orphanAfterMs = DEFAULT_ORPHAN_AFTER_MS, batch = 100 } = {}) {
  const result = { intentsClaimed: 0, orphanDeleted: 0, staleIntentsCleared: 0, orphanFailures: 0 };

  // Once the QH8Z ownership row exists, the create handoff completed and the
  // durable intent is no longer needed.
  const claimed = await pool.query(
    `DELETE FROM shlink_create_intents i
     USING links l
     WHERE l.short_code=i.short_code
     RETURNING i.short_code`
  );
  result.intentsClaimed = claimed.rows.length;

  const cutoff = new Date(Date.now() - Math.max(0, orphanAfterMs));
  const limit = Math.min(Math.max(Number(batch) || 100, 1), 500);
  const { rows } = await pool.query(
    `SELECT short_code,long_url,created_at
     FROM shlink_create_intents
     WHERE created_at <= $1
     ORDER BY created_at ASC
     LIMIT $2`,
    [cutoff, limit]
  );

  for (let offset = 0; offset < rows.length; offset += RECONCILE_CONCURRENCY) {
    const chunk = rows.slice(offset, offset + RECONCILE_CONCURRENCY);
    const outcomes = await Promise.all(chunk.map(async intent => {
      const link = { short_code: intent.short_code };
      let observed;
      try {
        observed = await observeUpstream(link);
      } catch (error) {
        console.warn(JSON.stringify({ level: 'warn', event: 'consistency.orphan_lookup_failed', shortCode: intent.short_code, message: error.message }));
        return 'failed';
      }

      if (!observed.found) {
        await pool.query('DELETE FROM shlink_create_intents WHERE short_code=$1', [intent.short_code]);
        return 'stale';
      }

      let deleted = false;
      try {
        await deleteShortUrl(intent.short_code);
        deleted = true;
      } catch (error) {
        if (error?.status === 404) {
          deleted = true;
        } else {
          try {
            const after = await observeUpstream(link);
            deleted = !after.found;
          } catch (verifyError) {
            console.error(JSON.stringify({ level: 'error', event: 'consistency.orphan_delete_unresolved', shortCode: intent.short_code, message: verifyError.message }));
          }
        }
      }

      if (!deleted) {
        console.error(JSON.stringify({ level: 'error', event: 'consistency.orphan_delete_failed', shortCode: intent.short_code }));
        return 'failed';
      }

      await pool.query('DELETE FROM shlink_create_intents WHERE short_code=$1', [intent.short_code]);
      await audit(null, 'consistency.orphan_short_deleted', intent.short_code, {
        shortCode: intent.short_code,
        expectedDestination: intent.long_url,
        observedDestination: observed.value?.longUrl || null,
      });
      return 'deleted';
    }));

    for (const outcome of outcomes) {
      if (outcome === 'deleted') result.orphanDeleted += 1;
      else if (outcome === 'stale') result.staleIntentsCleared += 1;
      else if (outcome === 'failed') result.orphanFailures += 1;
    }
  }

  return result;
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

  try {
    await editShortUrl(current.short_code, { longUrl: current.long_url, title: current.title });
  } catch (error) {
    console.warn(JSON.stringify({ level: 'warn', event: 'consistency.repair_request_failed', linkId: current.id, message: error.message }));
  }

  let repaired = false;
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

  let deleted = false;
  try {
    await deleteShortUrl(current.short_code);
    deleted = true;
  } catch (error) {
    if (error?.status === 404) {
      deleted = true;
    } else {
      try {
        const afterDelete = await observeUpstream(current);
        deleted = !afterDelete.found;
      } catch (verifyError) {
        console.error(JSON.stringify({ level: 'error', event: 'consistency.fail_closed_delete_unresolved', linkId: current.id, message: verifyError.message }));
      }
    }
  }

  if (!deleted) {
    console.error(JSON.stringify({ level: 'error', event: 'consistency.fail_closed_delete_failed', linkId: current.id }));
    return 'repair_failed';
  }
  await markMissing(current, 'persistent_upstream_mismatch');
  return 'disabled_mismatch';
}

export async function reconcileDueLinks({ confirmAfterMs = DEFAULT_CONFIRM_AFTER_MS, orphanAfterMs = DEFAULT_ORPHAN_AFTER_MS, batch = null } = {}) {
  const limit = Math.min(Math.max(Number(batch) || config.reputationRecheckBatch * 4 || 100, 25), 500);
  const intentResult = await reconcileCreateIntents({ orphanAfterMs, batch: limit });
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

  const results = { checked: 0, consistent: 0, repaired: 0, disabled: 0, pending: 0, failed: 0, ...intentResult };
  for (let offset = 0; offset < rows.length; offset += RECONCILE_CONCURRENCY) {
    const outcomes = await Promise.all(rows.slice(offset, offset + RECONCILE_CONCURRENCY).map(link => reconcileTrackedLink(link, { confirmAfterMs })));
    for (const outcome of outcomes) {
      results.checked += 1;
      if (outcome === 'consistent') results.consistent += 1;
      else if (outcome === 'repaired') results.repaired += 1;
      else if (outcome.startsWith('disabled_')) results.disabled += 1;
      else if (outcome === 'mismatch_pending') results.pending += 1;
      else if (outcome.endsWith('failed')) results.failed += 1;
    }
  }
  return results;
}
