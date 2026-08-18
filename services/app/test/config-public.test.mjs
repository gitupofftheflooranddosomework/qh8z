import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';

const base = {
  ...process.env,
  NODE_ENV: 'production',
  PUBLIC_LAUNCH_MODE: 'true',
  QH8Z_DOMAIN: 'qh8z.test',
  APP_BASE_URL: 'https://qh8z.test',
  PUBLIC_SHORT_BASE_URL: 'https://qh8z.test',
  COOKIE_SECURE: 'true',
  ALLOW_SIGNUP: 'true',
  EMAIL_VERIFICATION_REQUIRED: 'true',
  WEB_RISK_REQUIRED: 'true',
  WEB_RISK_API_KEY: 'test-web-risk-key',
  TURNSTILE_REQUIRED: 'true',
  TURNSTILE_SITE_KEY: 'test-site-key',
  TURNSTILE_SECRET_KEY: 'test-secret-key',
  MAIL_MODE: 'smtp',
  SMTP_HOST: 'smtp.qh8z.test',
  SMTP_PORT: '587',
  MAIL_FROM: 'QH8Z <support@qh8z.test>',
  SHLINK_API_KEY: 's'.repeat(32),
  ADMIN_EMAIL: 'admin@qh8z.test',
  ADMIN_BOOTSTRAP_SECRET: 'b'.repeat(32),
  PORT: '3000',
  SESSION_TTL_DAYS: '30',
  ADMIN_SESSION_HOURS: '12',
  DATA_RETENTION_DAYS: '365',
  REPUTATION_RECHECK_HOURS: '24',
  REPUTATION_RECHECK_BATCH: '25',
  REPUTATION_WORKER_MINUTES: '15',
  MFA_ENCRYPTION_KEY: 'ab'.repeat(32),
  TERMS_VERSION: '2026-08-17',
  SUPPORT_EMAIL: 'support@qh8z.test',
  ABUSE_EMAIL: 'abuse@qh8z.test',
  LEGAL_OPERATOR_NAME: 'QH8Z Test Operator',
  LEGAL_JURISDICTION: 'Test Jurisdiction',
  STRIPE_SECRET_KEY: '',
  STRIPE_WEBHOOK_SECRET: '',
  STRIPE_PRO_PRICE_ID: '',
};

function problems(overrides = {}) {
  const script = "import('./src/config.mjs').then(m=>process.stdout.write(JSON.stringify(m.startupProblems())))";
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', script], { cwd: new URL('..', import.meta.url), env: { ...base, ...overrides }, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test('complete public configuration passes static launch checks', () => assert.deepEqual(problems(), []));
test('post-bootstrap configuration may remove the one-time bootstrap secret', () => assert.deepEqual(problems({ ADMIN_BOOTSTRAP_SECRET: '' }), []));
test('public security mode stays valid while new signup is temporarily closed', () => assert.deepEqual(problems({ ALLOW_SIGNUP: 'false' }), []));
test('public launch refuses missing Turnstile', () => assert.ok(problems({ TURNSTILE_SECRET_KEY: '' }).some(x => x.includes('Turnstile'))));
test('public launch refuses placeholder admin identity', () => assert.ok(problems({ ADMIN_EMAIL: 'admin@example.example' }).some(x => x.includes('ADMIN_EMAIL'))));
test('public launch refuses insecure cookie mode', () => assert.ok(problems({ COOKIE_SECURE: 'false' }).some(x => x.includes('COOKIE_SECURE'))));
test('public launch requires origin-only base URLs', () => {
  assert.ok(problems({ APP_BASE_URL: 'https://qh8z.test/app' }).some(x => x.includes('APP_BASE_URL')));
  assert.ok(problems({ PUBLIC_SHORT_BASE_URL: 'https://qh8z.test/s' }).some(x => x.includes('PUBLIC_SHORT_BASE_URL')));
  assert.ok(problems({ APP_BASE_URL: 'https://user:pass@qh8z.test' }).some(x => x.includes('APP_BASE_URL')));
  assert.ok(problems({ PUBLIC_SHORT_BASE_URL: 'https://qh8z.test?x=1' }).some(x => x.includes('PUBLIC_SHORT_BASE_URL')));
});
test('public launch binds generated URLs to the configured Caddy host', () => {
  assert.ok(problems({ QH8Z_DOMAIN: '' }).some(x => x.includes('QH8Z_DOMAIN')));
  assert.ok(problems({ QH8Z_DOMAIN: 'qh8z.test/path' }).some(x => x.includes('QH8Z_DOMAIN')));
  assert.ok(problems({ APP_BASE_URL: 'https://other.test' }).some(x => x.includes('APP_BASE_URL host')));
  assert.ok(problems({ PUBLIC_SHORT_BASE_URL: 'https://other.test' }).some(x => x.includes('PUBLIC_SHORT_BASE_URL host')));
});
test('public launch rejects malformed numeric values instead of accepting prefixes', () => {
  assert.ok(problems({ PORT: '3000oops' }).some(x => x.includes('PORT')));
  assert.ok(problems({ SMTP_PORT: '587tls' }).some(x => x.includes('SMTP_PORT')));
  assert.ok(problems({ SESSION_TTL_DAYS: '30days' }).some(x => x.includes('SESSION_TTL_DAYS')));
  assert.ok(problems({ REPUTATION_WORKER_MINUTES: '15m' }).some(x => x.includes('REPUTATION_WORKER_MINUTES')));
});
test('public launch rejects invalid user and admin session lifetimes', () => {
  assert.ok(problems({ SESSION_TTL_DAYS: '0' }).some(x => x.includes('SESSION_TTL_DAYS')));
  assert.ok(problems({ ADMIN_SESSION_HOURS: '48' }).some(x => x.includes('ADMIN_SESSION_HOURS')));
});
test('public launch cannot silently disable recurring reputation scanning', () => {
  assert.ok(problems({ REPUTATION_RECHECK_HOURS: '0' }).some(x => x.includes('REPUTATION_RECHECK_HOURS')));
  assert.ok(problems({ REPUTATION_RECHECK_BATCH: '0' }).some(x => x.includes('REPUTATION_RECHECK_BATCH')));
  assert.ok(problems({ REPUTATION_WORKER_MINUTES: '0' }).some(x => x.includes('REPUTATION_WORKER_MINUTES')));
});
test('public launch rejects unsafe retention bounds', () => assert.ok(problems({ DATA_RETENTION_DAYS: '1' }).some(x => x.includes('DATA_RETENTION_DAYS'))));
test('public launch rejects malformed contact and legal presentation values', () => {
  assert.ok(problems({ SUPPORT_EMAIL: 'support@qh8z.test<script>' }).some(x => x.includes('Support and abuse')));
  assert.ok(problems({ LEGAL_OPERATOR_NAME: '<b>QH8Z</b>' }).some(x => x.includes('plain text')));
});
