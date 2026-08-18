export const RESERVED_SLUGS = new Set([
  'api', 'app', 'login', 'signup', 'admin', 'pricing', 'report', 'healthz', 'readyz',
  'verify', 'forgot', 'reset', 'privacy', 'terms', 'security', 'assets', 'favicon.svg',
  'robots.txt', 'www', 'status', 'support', 'abuse', 'rest'
]);

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
  if (!raw || raw.length > 8192) throw new Error('Destination URLs must be between 1 and 8192 characters');
  const parsed = new URL(raw);
  if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error('Only http and https URLs are allowed');
  if (parsed.username || parsed.password) throw new Error('URLs containing embedded credentials are not allowed');
  return parsed.toString();
}

export function normalizeSlug(value) {
  if (value == null || String(value).trim() === '') return null;
  const slug = String(value).trim();
  if (!/^[A-Za-z0-9_-]{3,64}$/.test(slug)) throw new Error('Custom aliases must be 3-64 characters using letters, numbers, hyphens, or underscores');
  if (RESERVED_SLUGS.has(slug.toLowerCase())) throw new Error('That alias is reserved by QH8Z');
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
