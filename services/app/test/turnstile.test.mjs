import test from 'node:test';
import assert from 'node:assert/strict';

process.env.NODE_ENV = 'production';
process.env.APP_BASE_URL = 'https://qh8z.test';
process.env.TURNSTILE_SECRET_KEY = 'unit-test-secret';
process.env.TURNSTILE_REQUIRED = 'true';

const originalFetch = globalThis.fetch;
const { verifyTurnstile } = await import('../src/turnstile.mjs');

test.after(() => { globalThis.fetch = originalFetch; });

test('accepts successful Turnstile response for expected action and hostname', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ success: true, action: 'signup', hostname: 'qh8z.test' }), { status: 200, headers: { 'content-type': 'application/json' } });
  const result = await verifyTurnstile('token', 'signup', '203.0.113.10');
  assert.equal(result.success, true);
});

test('rejects action mismatch', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ success: true, action: 'login', hostname: 'qh8z.test' }), { status: 200, headers: { 'content-type': 'application/json' } });
  await assert.rejects(() => verifyTurnstile('token', 'signup', '203.0.113.10'), /Human verification failed/);
});

test('rejects hostname mismatch in production', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ success: true, action: 'signup', hostname: 'attacker.test' }), { status: 200, headers: { 'content-type': 'application/json' } });
  await assert.rejects(() => verifyTurnstile('token', 'signup', '203.0.113.10'), /Human verification failed/);
});
