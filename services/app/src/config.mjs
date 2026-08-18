const bool = (value, fallback = false) => {
  if (value == null) return fallback;
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
};

const int = (value, fallback) => {
  if (value == null || String(value).trim() === '') return fallback;
  const raw = String(value).trim();
  if (!/^-?\d+$/.test(raw)) return Number.NaN;
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) ? parsed : Number.NaN;
};

const trimmed = (value, fallback = '') => String(value ?? fallback).trim();
const emailLike = value => {
  const email = trimmed(value).toLowerCase();
  return email.length <= 254 && /^[^\s<>@]+@[^\s<>@]+\.[^\s<>@]+$/.test(email);
};
const mailFromLike = value => {
  const raw = trimmed(value);
  if (!raw || /[\r\n]/.test(raw)) return false;
  const bracketed = raw.match(/^(?:[^<>]{0,120})<([^<>]+)>$/);
  return emailLike(bracketed ? bracketed[1] : raw);
};
const placeholderLike = value => /(?:replace-me|replace-with|change-me)/i.test(String(value || ''));
const integerBetween = (value, min, max) => Number.isInteger(value) && value >= min && value <= max;
const hostnameLike = value => {
  const host = trimmed(value).toLowerCase();
  return host.length <= 253 && /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$/.test(host);
};
const RESERVED_PUBLIC_SUFFIXES = ['localhost', 'local', 'test', 'invalid', 'example', 'internal'];
const publicHostnameLike = value => {
  const host = trimmed(value).toLowerCase().replace(/\.$/, '');
  if (!hostnameLike(host) || !host.includes('.')) return false;
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(host)) return false;
  return !RESERVED_PUBLIC_SUFFIXES.some(suffix => host === suffix || host.endsWith(`.${suffix}`));
};

export const config = Object.freeze({
  env: process.env.NODE_ENV || 'development',
  port: int(process.env.PORT, 3000),
  databaseUrl: process.env.DATABASE_URL || 'postgres://qh8z:qh8z@db:5432/qh8z',
  qh8zDomain: trimmed(process.env.QH8Z_DOMAIN).toLowerCase(),
  appBaseUrl: (process.env.APP_BASE_URL || 'http://localhost:3000').replace(/\/$/, ''),
  publicShortBaseUrl: (process.env.PUBLIC_SHORT_BASE_URL || 'http://localhost:8080').replace(/\/$/, ''),
  shlinkBaseUrl: (process.env.SHLINK_BASE_URL || 'http://shlink:8080').replace(/\/$/, ''),
  shlinkApiKey: process.env.SHLINK_API_KEY || '',
  cookieSecure: bool(process.env.COOKIE_SECURE, process.env.NODE_ENV === 'production'),
  sessionTtlDays: int(process.env.SESSION_TTL_DAYS, 30),
  adminSessionHours: int(process.env.ADMIN_SESSION_HOURS, 12),
  adminEmail: trimmed(process.env.ADMIN_EMAIL).toLowerCase(),
  adminBootstrapSecret: process.env.ADMIN_BOOTSTRAP_SECRET || '',
  mfaEncryptionKey: process.env.MFA_ENCRYPTION_KEY || '',
  allowSignup: bool(process.env.ALLOW_SIGNUP, true),
  publicLaunchMode: bool(process.env.PUBLIC_LAUNCH_MODE, false),
  emailVerificationRequired: bool(process.env.EMAIL_VERIFICATION_REQUIRED, process.env.NODE_ENV === 'production'),
  termsVersion: trimmed(process.env.TERMS_VERSION, '2026-08-17'),
  authTokenExposeInDev: bool(process.env.DEV_EXPOSE_AUTH_TOKENS, false) && process.env.NODE_ENV !== 'production',
  stripeSecretKey: process.env.STRIPE_SECRET_KEY || '',
  stripeWebhookSecret: process.env.STRIPE_WEBHOOK_SECRET || '',
  stripeProPriceId: process.env.STRIPE_PRO_PRICE_ID || '',
  supportEmail: trimmed(process.env.SUPPORT_EMAIL, 'support@qh8z.com'),
  abuseEmail: trimmed(process.env.ABUSE_EMAIL, process.env.SUPPORT_EMAIL || 'abuse@qh8z.com'),
  webRiskApiKey: process.env.WEB_RISK_API_KEY || '',
  webRiskRequired: bool(process.env.WEB_RISK_REQUIRED, process.env.NODE_ENV === 'production'),
  turnstileSiteKey: process.env.TURNSTILE_SITE_KEY || '',
  turnstileSecretKey: process.env.TURNSTILE_SECRET_KEY || '',
  turnstileRequired: bool(process.env.TURNSTILE_REQUIRED, process.env.NODE_ENV === 'production'),
  mailMode: trimmed(process.env.MAIL_MODE, process.env.NODE_ENV === 'production' ? 'smtp' : 'log').toLowerCase(),
  mailFrom: trimmed(process.env.MAIL_FROM, 'QH8Z <support@qh8z.com>'),
  smtpHost: trimmed(process.env.SMTP_HOST),
  smtpPort: int(process.env.SMTP_PORT, 587),
  smtpSecure: bool(process.env.SMTP_SECURE, false),
  smtpRequireTls: bool(process.env.SMTP_REQUIRE_TLS, true),
  smtpUser: trimmed(process.env.SMTP_USER),
  smtpPass: process.env.SMTP_PASS || '',
  retentionDays: int(process.env.DATA_RETENTION_DAYS, 365),
  reputationRecheckHours: int(process.env.REPUTATION_RECHECK_HOURS, 24),
  reputationRecheckBatch: int(process.env.REPUTATION_RECHECK_BATCH, 25),
  reputationWorkerMinutes: int(process.env.REPUTATION_WORKER_MINUTES, 15),
  legalOperatorName: trimmed(process.env.LEGAL_OPERATOR_NAME),
  legalJurisdiction: trimmed(process.env.LEGAL_JURISDICTION),
});

export const plans = Object.freeze({
  free: { name: 'Free', links: 25, customSlugs: true, priceLabel: '$0' },
  pro: { name: 'Pro', links: 5000, customSlugs: true, priceLabel: '$6/mo' },
});

function requireHttpsOrigin(problems, name, value) {
  try {
    const url = new URL(value);
    if (url.protocol !== 'https:') {
      problems.push(`${name} must use HTTPS`);
      return null;
    }
    if (url.username || url.password || url.pathname !== '/' || url.search || url.hash || url.port) {
      problems.push(`${name} must be a standard-port HTTPS origin without a path, query, fragment, or embedded credentials`);
      return null;
    }
    return url;
  } catch {
    problems.push(`${name} must be a valid absolute HTTPS origin`);
    return null;
  }
}

export function startupProblems() {
  const problems = [];
  const billingValues = [config.stripeSecretKey, config.stripeWebhookSecret, config.stripeProPriceId];
  if (billingValues.some(Boolean) && !billingValues.every(Boolean)) problems.push('Stripe must be configured with secret key, webhook secret, and Pro price ID together');
  if (!config.publicLaunchMode) return problems;
  if (config.env !== 'production') problems.push('PUBLIC_LAUNCH_MODE requires NODE_ENV=production');
  if (!publicHostnameLike(config.qh8zDomain)) problems.push('QH8Z_DOMAIN must be a public fully qualified hostname, not localhost, an IP address, or a reserved test/internal name');
  const appOrigin = requireHttpsOrigin(problems, 'APP_BASE_URL', config.appBaseUrl);
  const shortOrigin = requireHttpsOrigin(problems, 'PUBLIC_SHORT_BASE_URL', config.publicShortBaseUrl);
  if (publicHostnameLike(config.qh8zDomain) && appOrigin && appOrigin.hostname.toLowerCase() !== config.qh8zDomain) problems.push('APP_BASE_URL host must exactly match QH8Z_DOMAIN');
  if (publicHostnameLike(config.qh8zDomain) && shortOrigin && shortOrigin.hostname.toLowerCase() !== config.qh8zDomain) problems.push('PUBLIC_SHORT_BASE_URL host must exactly match QH8Z_DOMAIN');
  if (!integerBetween(config.port, 1, 65535)) problems.push('PORT must be an integer between 1 and 65535');
  if (!config.cookieSecure) problems.push('COOKIE_SECURE must be true');
  if (!config.emailVerificationRequired) problems.push('EMAIL_VERIFICATION_REQUIRED must be true');
  if (!config.webRiskRequired || !config.webRiskApiKey) problems.push('Google Web Risk must be configured and required');
  if (!config.turnstileRequired || !config.turnstileSiteKey || !config.turnstileSecretKey) problems.push('Cloudflare Turnstile must be configured and required');
  if (config.mailMode !== 'smtp' || !config.smtpHost || !config.mailFrom) problems.push('SMTP email delivery must be configured');
  if (!mailFromLike(config.mailFrom)) problems.push('MAIL_FROM must contain a valid sender email address without control characters');
  const smtpUserSet = Boolean(config.smtpUser);
  const smtpPassSet = Boolean(config.smtpPass);
  if (smtpUserSet !== smtpPassSet) problems.push('SMTP_USER and SMTP_PASS must be configured together or both left blank for an unauthenticated relay');
  if (placeholderLike(config.smtpUser) || placeholderLike(config.smtpPass)) problems.push('SMTP credentials must not contain template placeholder values');
  if (!integerBetween(config.smtpPort, 1, 65535)) problems.push('SMTP_PORT must be an integer between 1 and 65535');
  if (!config.shlinkApiKey || config.shlinkApiKey.length < 24) problems.push('SHLINK_API_KEY must be a strong secret');
  if (!emailLike(config.adminEmail) || config.adminEmail.endsWith('.example') || config.adminEmail.includes('replace-with')) problems.push('ADMIN_EMAIL must be the real administrator email');
  if (!/^[0-9a-fA-F]{64}$/.test(config.mfaEncryptionKey)) problems.push('MFA_ENCRYPTION_KEY must be 32 random bytes encoded as 64 hex characters');
  if (!integerBetween(config.sessionTtlDays, 1, 90)) problems.push('SESSION_TTL_DAYS must be an integer between 1 and 90');
  if (!integerBetween(config.adminSessionHours, 1, 24)) problems.push('ADMIN_SESSION_HOURS must be an integer between 1 and 24');
  if (!integerBetween(config.retentionDays, 30, 3650)) problems.push('DATA_RETENTION_DAYS must be an integer between 30 and 3650');
  if (!integerBetween(config.reputationRecheckHours, 1, 168)) problems.push('REPUTATION_RECHECK_HOURS must be an integer between 1 and 168 in public mode');
  if (!integerBetween(config.reputationRecheckBatch, 1, 1000)) problems.push('REPUTATION_RECHECK_BATCH must be an integer between 1 and 1000 in public mode');
  if (!integerBetween(config.reputationWorkerMinutes, 1, 60)) problems.push('REPUTATION_WORKER_MINUTES must be an integer between 1 and 60 in public mode');
  if (!config.termsVersion) problems.push('TERMS_VERSION is required');
  if (!emailLike(config.supportEmail) || !emailLike(config.abuseEmail)) problems.push('Support and abuse email addresses must be valid');
  if (!config.legalOperatorName || !config.legalJurisdiction) problems.push('LEGAL_OPERATOR_NAME and LEGAL_JURISDICTION must identify the public service operator');
  if (/[<>]/.test(config.legalOperatorName) || /[<>]/.test(config.legalJurisdiction)) problems.push('LEGAL_OPERATOR_NAME and LEGAL_JURISDICTION must be plain text without HTML markup');
  return problems;
}
