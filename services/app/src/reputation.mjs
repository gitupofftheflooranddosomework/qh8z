import { config } from './config.mjs';
import { pool, audit } from './db.mjs';
import { deleteShortUrl } from './shlink.mjs';
import { assertDestinationAllowed } from './destination.mjs';

const THREAT_TYPES = ['MALWARE', 'SOCIAL_ENGINEERING', 'UNWANTED_SOFTWARE'];

export async function checkUrlReputation(url) {
  if (!config.webRiskApiKey) {
    if (config.webRiskRequired) {
      const error = new Error('URL reputation checking is required but WEB_RISK_API_KEY is not configured');
      error.status = 503;
      throw error;
    }
    return { checked: false, threats: [] };
  }

  const params = new URLSearchParams({ uri: url, key: config.webRiskApiKey });
  for (const threatType of THREAT_TYPES) params.append('threatTypes', threatType);

  try {
    const response = await fetch(`https://webrisk.googleapis.com/v1/uris:search?${params}`, {
      method: 'GET',
      signal: AbortSignal.timeout(5000),
      headers: { accept: 'application/json' },
    });
    if (!response.ok) {
      const error = new Error(`Web Risk returned ${response.status}`);
      error.status = 503;
      throw error;
    }
    const body = await response.json();
    return { checked: true, threats: body?.threat?.threatTypes || [] };
  } catch (error) {
    if (config.webRiskRequired) {
      error.status = 503;
      throw error;
    }
    console.warn(JSON.stringify({ level: 'warn', event: 'reputation.unavailable', message: error.message }));
    return { checked: false, threats: [] };
  }
}

async function recheckDueLinks() {
  if (!config.webRiskApiKey || config.reputationRecheckHours <= 0) return;
  const { rows } = await pool.query(
    `SELECT id,short_code,long_url FROM links
     WHERE disabled_at IS NULL
       AND (reputation_checked_at IS NULL OR reputation_checked_at < updated_at OR reputation_checked_at < NOW() - ($1::text || ' hours')::interval)
     ORDER BY reputation_checked_at ASC NULLS FIRST, updated_at ASC
     LIMIT $2`,
    [config.reputationRecheckHours, config.reputationRecheckBatch]
  );
  for (const link of rows) {
    try {
      assertDestinationAllowed(link.long_url);
      const result = await checkUrlReputation(link.long_url);
      if (result.threats.length) {
        try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
        await pool.query("UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),reputation_checked_at=NOW(),reputation_status='blocked',updated_at=NOW() WHERE id=$1", [link.id]);
        await audit(null, 'reputation.link_blocked', link.id, { shortCode: link.short_code, threats: result.threats });
      } else if (result.checked) {
        await pool.query("UPDATE links SET reputation_checked_at=NOW(),reputation_status='clean' WHERE id=$1", [link.id]);
      }
    } catch (error) {
      if (/Local-network|Private or reserved|short domain/.test(error.message || '')) {
        try { await deleteShortUrl(link.short_code); } catch (deleteError) { if (deleteError?.status !== 404) console.error(deleteError); }
        await pool.query("UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),reputation_checked_at=NOW(),reputation_status='blocked',updated_at=NOW() WHERE id=$1", [link.id]);
        await audit(null, 'reputation.link_blocked_policy', link.id, { shortCode: link.short_code });
      } else {
        console.warn(JSON.stringify({ level: 'warn', event: 'reputation.recheck_failed', linkId: link.id, message: error.message }));
      }
    }
  }
}

let reputationWorkerStarted = false;
export function startReputationWorker() {
  if (reputationWorkerStarted || config.reputationWorkerMinutes <= 0) return;
  reputationWorkerStarted = true;
  const run = () => recheckDueLinks().catch(error => console.error(JSON.stringify({ level: 'error', event: 'reputation.worker_failed', message: error.message })));
  setTimeout(run, 30_000).unref();
  setInterval(run, config.reputationWorkerMinutes * 60_000).unref();
}
