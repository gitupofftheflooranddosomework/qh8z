import { config } from './config.mjs';

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

export function createShortUrl({ longUrl, customSlug, title }) {
  return request('/rest/v3/short-urls', {
    method: 'POST',
    body: JSON.stringify({ longUrl, ...(customSlug ? { customSlug } : {}), ...(title ? { title } : {}), findIfExists: false, crawlable: false, forwardQuery: true })
  });
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
