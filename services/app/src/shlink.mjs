import crypto from 'node:crypto';
import { config } from './config.mjs';
import { pool } from './db.mjs';
import { RESERVED_SLUGS } from './validation.mjs';

const CODE_ALPHABET = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';

function headers() {
  if (!config.shlinkApiKey) throw new Error('SHLINK_API_KEY is not configured');
  return { 'content-type': 'application/json', accept: 'application/json', 'X-Api-Key': config.shlinkApiKey };
}

async function request(path, options = {}) {
  const response = await fetch(`${config.shlinkBaseUrl}${path}`, { ...options, headers: { ...headers(), ...(options.headers || {}) }, signal: options.signal || AbortSignal.timeout(8000) });
  const text = await response.text();
  let body = null;
  if (text) {
    try { body = JSON.parse(text); } catch { body = { raw: text }; }
  }
  if (!response.ok) {
    const error = new Error(body?.detail || body?.title || `Shlink returned ${response.status}`);
    error.status = response.status;
    error.upstream = body;
    throw error;
  }
  return body;
}

export async function checkShlinkHealth() {
  const response = await fetch(`${config.shlinkBaseUrl}/rest/health`, { headers: { accept: 'application/json' }, signal: AbortSignal.timeout(5000) });
  return response.ok;
}

export function generateShortCode(length = 7) {
  let code = '';
  for (let i = 0; i < length; i += 1) code += CODE_ALPHABET[crypto.randomInt(CODE_ALPHABET.length)];
  return code;
}

async function claimCreateIntent(shortCode, longUrl) {
  const { rows } = await pool.query(
    `INSERT INTO shlink_create_intents(short_code,long_url)
     VALUES($1,$2)
     ON CONFLICT (short_code) DO NOTHING
     RETURNING short_code`,
    [shortCode, longUrl]
  );
  return Boolean(rows[0]);
}

async function clearCreateIntent(shortCode) {
  await pool.query('DELETE FROM shlink_create_intents WHERE short_code=$1', [shortCode]);
}

async function observeShortCode(shortCode) {
  try {
    return { found: true, value: await getShortUrl(shortCode) };
  } catch (error) {
    if (error?.status === 404) return { found: false, value: null };
    throw error;
  }
}

function sameDestination(a, b) {
  try { return new URL(String(a)).toString() === new URL(String(b)).toString(); } catch { return false; }
}

export async function createShortUrl({ longUrl, customSlug, title }) {
  const suppliedSlug = Boolean(customSlug);
  const maxAttempts = suppliedSlug ? 1 : 8;

  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    const candidate = customSlug || generateShortCode();
    if (!suppliedSlug && RESERVED_SLUGS.has(candidate.toLowerCase())) continue;
    if (!(await claimCreateIntent(candidate, longUrl))) {
      if (!suppliedSlug) continue;
      const error = new Error('That alias is already being created');
      error.status = 409;
      throw error;
    }

    let postAttempted = false;
    try {
      // QH8Z ownership wins even if the redirect engine temporarily lost the
      // corresponding short URL. Never recreate an alias already present in the
      // product database for another request/user and rely on a later unique
      // constraint to catch it—that would create a brief wrong-target window.
      const owned = await pool.query('SELECT id FROM links WHERE short_code=$1 LIMIT 1', [candidate]);
      if (owned.rows[0]) {
        await clearCreateIntent(candidate);
        if (!suppliedSlug) continue;
        const error = new Error('That alias already exists');
        error.status = 409;
        throw error;
      }

      const before = await observeShortCode(candidate);
      if (before.found) {
        await clearCreateIntent(candidate);
        if (!suppliedSlug) continue;
        const error = new Error('That alias already exists');
        error.status = 409;
        throw error;
      }

      postAttempted = true;
      const created = await request('/rest/v3/short-urls', {
        method: 'POST',
        body: JSON.stringify({ longUrl, customSlug: candidate, ...(title ? { title } : {}), findIfExists: false, crawlable: false, forwardQuery: true })
      });
      return { ...created, shortCode: created?.shortCode || candidate };
    } catch (error) {
      if (error?.status) {
        await clearCreateIntent(candidate);
        if (!suppliedSlug && [400, 409].includes(error.status)) continue;
        throw error;
      }

      if (!postAttempted) {
        await clearCreateIntent(candidate);
        throw error;
      }

      try {
        const after = await observeShortCode(candidate);
        if (after.found && sameDestination(after.value?.longUrl, longUrl)) {
          return { ...after.value, shortCode: after.value?.shortCode || candidate };
        }
        if (!after.found) await clearCreateIntent(candidate);
      } catch (lookupError) {
        console.warn(JSON.stringify({ level: 'warn', event: 'shlink.create_ambiguous', shortCode: candidate, message: lookupError.message }));
      }
      throw error;
    }
  }

  const error = new Error('Could not allocate an unused short code');
  error.status = 503;
  throw error;
}

export function getShortUrl(shortCode) {
  return request(`/rest/v3/short-urls/${encodeURIComponent(shortCode)}`, { method: 'GET' });
}

export function editShortUrl(shortCode, { longUrl, title }) {
  return request(`/rest/v3/short-urls/${encodeURIComponent(shortCode)}`, { method: 'PATCH', body: JSON.stringify({ longUrl, title: title ?? null }) });
}

export function deleteShortUrl(shortCode) {
  return request(`/rest/v3/short-urls/${encodeURIComponent(shortCode)}`, { method: 'DELETE' });
}

export function getVisits(shortCode, page = 1, itemsPerPage = 50) {
  const query = new URLSearchParams({ page: String(page), itemsPerPage: String(itemsPerPage) });
  return request(`/rest/v3/short-urls/${encodeURIComponent(shortCode)}/visits?${query}`, { method: 'GET' });
}
