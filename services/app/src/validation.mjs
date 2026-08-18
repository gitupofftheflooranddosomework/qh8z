export const RESERVED_SLUGS = new Set([
  'api', 'app', 'login', 'signup', 'admin', 'pricing', 'report', 'healthz', 'readyz',
  'verify', 'forgot', 'reset', 'privacy', 'terms', 'security', 'assets', 'favicon.svg',
  'robots.txt', 'www', 'status', 'support', 'abuse', 'rest'
]);

function clientError(message, code = 'invalid_input') {
  const error = new Error(message);
  error.status = 400;
  error.code = code;
  return error;
}

export function normalizeEmail(value) {
  return String(value || '').trim().toLowerCase();
}

export function validEmail(value) {
  const email = normalizeEmail(value);
  return email.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export function validPassword(value) {
  return typeof value === 'string' && value.length >= 10 && Buffer.byteLength(value, 'utf8') <= 72;
}

export function normalizeHttpUrl(value) {
  const raw = String(value || '').trim();
  if (!raw || raw.length > 8192) throw clientError('Destination URLs must be between 1 and 8192 characters', 'invalid_destination');
  let parsed;
  try { parsed = new URL(raw); }
  catch { throw clientError('Destination must be a valid absolute URL', 'invalid_destination'); }
  if (!['http:', 'https:'].includes(parsed.protocol)) throw clientError('Only http and https URLs are allowed', 'invalid_destination');
  if (parsed.username || parsed.password) throw clientError('URLs containing embedded credentials are not allowed', 'invalid_destination');
  return parsed.toString();
}

export function normalizeSlug(value) {
  if (value == null || String(value).trim() === '') return null;
  const slug = String(value).trim();
  if (!/^[A-Za-z0-9_-]{3,64}$/.test(slug)) throw clientError('Custom aliases must be 3-64 characters using letters, numbers, hyphens, or underscores', 'invalid_alias');
  if (RESERVED_SLUGS.has(slug.toLowerCase())) throw clientError('That alias is reserved by QH8Z', 'reserved_alias');
  return slug;
}

export function cleanTitle(value) {
  if (value == null) return null;
  const title = String(value).trim();
  return title ? title.slice(0, 160) : null;
}

export function accepted(value) {
  return value === true || ['1','true','yes','on'].includes(String(value || '').toLowerCase());
}
