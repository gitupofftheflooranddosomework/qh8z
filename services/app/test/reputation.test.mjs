import test from 'node:test';
import assert from 'node:assert/strict';

process.env.WEB_RISK_API_KEY = 'unit-test-key';
process.env.WEB_RISK_REQUIRED = 'true';
process.env.REPUTATION_WORKER_MINUTES = '0';

const originalFetch = globalThis.fetch;
const { checkUrlReputation } = await import('../src/reputation.mjs');

test.after(() => { globalThis.fetch = originalFetch; });

test('accepts a clean Web Risk response', async () => {
  globalThis.fetch = async () => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
  assert.deepEqual(await checkUrlReputation('https://example.com'), { checked: true, threats: [] });
});

test('surfaces Web Risk threat types', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ threat: { threatTypes: ['SOCIAL_ENGINEERING'] } }), { status: 200, headers: { 'content-type': 'application/json' } });
  const result = await checkUrlReputation('https://example.com');
  assert.deepEqual(result.threats, ['SOCIAL_ENGINEERING']);
});

test('fails closed when required Web Risk is unavailable', async () => {
  globalThis.fetch = async () => new Response('{}', { status: 503 });
  await assert.rejects(() => checkUrlReputation('https://example.com'), /Web Risk returned 503/);
});
