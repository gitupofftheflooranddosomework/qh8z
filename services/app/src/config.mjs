const bool = (value, fallback = false) => {
  if (value == null) return fallback;
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
};

const int = (value, fallback) => {
  const parsed = Number.parseInt(value ?? '', 10);
  return Number.isFinite(parsed) ? parsed : fallback;
};

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
  adminEmail: (process.env.ADMIN_EMAIL || '').trim().toLowerCase(),
  adminBootstrapSecret: process.env.ADMIN_BOOTSTRAP_SECRET || '',
  allowSignup: bool(process.env.ALLOW_SIGNUP, true),
  stripeSecretKey: process.env.STRIPE_SECRET_KEY || '',
  stripeWebhookSecret: process.env.STRIPE_WEBHOOK_SECRET || '',
  stripeProPriceId: process.env.STRIPE_PRO_PRICE_ID || '',
  supportEmail: process.env.SUPPORT_EMAIL || 'support@qh8z.com',
  webRiskApiKey: process.env.WEB_RISK_API_KEY || '',
  webRiskRequired: bool(process.env.WEB_RISK_REQUIRED, process.env.NODE_ENV === 'production'),
});

export const plans = Object.freeze({
  free: { name: 'Free', links: 25, customSlugs: true, priceLabel: '$0' },
  pro: { name: 'Pro', links: 5000, customSlugs: true, priceLabel: '$6/mo' },
});
