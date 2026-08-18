const bool = (value, fallback = false) => {
  if (value == null) return fallback;
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
};

const int = (value, fallback) => {
  const parsed = Number.parseInt(value ?? '', 10);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const trimmed = (value, fallback = '') => String(value ?? fallback).trim();

export const config = Object.freeze({
  env: process.env.NODE_ENV || 'development',
  port: int(process.env.PORT, 3000),
  databaseUrl: process.env.DATABASE_URL || 'postgres://qh8z:qh8z@db:5432/qh8z',
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
  smtpUser: process.env.SMTP_USER || '',
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

export function startupProblems() {
  const problems = [];
  const billingValues = [config.stripeSecretKey, config.stripeWebhookSecret, config.stripeProPriceId];
  if (billingValues.some(Boolean) && !billingValues.every(Boolean)) problems.push('Stripe must be configured with secret key, webhook secret, and Pro price ID together');
  if (!config.publicLaunchMode) return problems;
  if (config.env !== 'production') problems.push('PUBLIC_LAUNCH_MODE requires NODE_ENV=production');
  try {
    if (new URL(config.appBaseUrl).protocol !== 'https:') problems.push('APP_BASE_URL must use HTTPS');
    if (new URL(config.publicShortBaseUrl).protocol !== 'https:') problems.push('PUBLIC_SHORT_BASE_URL must use HTTPS');
  } catch {
    problems.push('APP_BASE_URL and PUBLIC_SHORT_BASE_URL must be valid absolute URLs');
  }
  if (!config.cookieSecure) problems.push('COOKIE_SECURE must be true');
  if (!config.emailVerificationRequired) problems.push('EMAIL_VERIFICATION_REQUIRED must be true');
  if (!config.webRiskRequired || !config.webRiskApiKey) problems.push('Google Web Risk must be configured and required');
  if (!config.turnstileRequired || !config.turnstileSiteKey || !config.turnstileSecretKey) problems.push('Cloudflare Turnstile must be configured and required');
  if (config.mailMode !== 'smtp' || !config.smtpHost || !config.mailFrom) problems.push('SMTP email delivery must be configured');
  if (!config.shlinkApiKey || config.shlinkApiKey.length < 24) problems.push('SHLINK_API_KEY must be a strong secret');
  if (!config.adminEmail || !config.adminEmail.includes('@') || config.adminEmail.endsWith('.example') || config.adminEmail.includes('replace-with')) problems.push('ADMIN_EMAIL must be the real administrator email');
  if (!/^[0-9a-fA-F]{64}$/.test(config.mfaEncryptionKey)) problems.push('MFA_ENCRYPTION_KEY must be 32 random bytes encoded as 64 hex characters');
  if (config.sessionTtlDays < 1 || config.sessionTtlDays > 90) problems.push('SESSION_TTL_DAYS must be between 1 and 90');
  if (config.adminSessionHours < 1 || config.adminSessionHours > 24) problems.push('ADMIN_SESSION_HOURS must be between 1 and 24');
  if (config.retentionDays < 30 || config.retentionDays > 3650) problems.push('DATA_RETENTION_DAYS must be between 30 and 3650');
  if (config.reputationRecheckHours < 1 || config.reputationRecheckHours > 168) problems.push('REPUTATION_RECHECK_HOURS must be between 1 and 168 in public mode');
  if (config.reputationRecheckBatch < 1 || config.reputationRecheckBatch > 1000) problems.push('REPUTATION_RECHECK_BATCH must be between 1 and 1000 in public mode');
  if (config.reputationWorkerMinutes < 1 || config.reputationWorkerMinutes > 60) problems.push('REPUTATION_WORKER_MINUTES must be between 1 and 60 in public mode');
  if (!config.termsVersion) problems.push('TERMS_VERSION is required');
  if (!config.supportEmail.includes('@') || !config.abuseEmail.includes('@')) problems.push('Support and abuse email addresses must be configured');
  if (!config.legalOperatorName || !config.legalJurisdiction) problems.push('LEGAL_OPERATOR_NAME and LEGAL_JURISDICTION must identify the public service operator');
  return problems;
}
