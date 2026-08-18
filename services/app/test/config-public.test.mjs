import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';

const base = {
  ...process.env,
  NODE_ENV: 'production',
  PUBLIC_LAUNCH_MODE: 'true',
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
  MAIL_FROM: 'QH8Z <support@qh8z.test>',
  SHLINK_API_KEY: 's'.repeat(32),
  ADMIN_EMAIL: 'admin@qh8z.test',
  ADMIN_BOOTSTRAP_SECRET: 'b'.repeat(32),
  ADMIN_SESSION_HOURS: '12',
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
test('public launch rejects overly long admin sessions', () => assert.ok(problems({ ADMIN_SESSION_HOURS: '48' }).some(x => x.includes('ADMIN_SESSION_HOURS'))));
