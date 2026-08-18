import test from 'node:test';
import assert from 'node:assert/strict';
import { pool } from '../src/db.mjs';
import { createShortUrl, generateShortCode } from '../src/shlink.mjs';

test('generated short codes stay compact, alphanumeric, and high-entropy', () => {
  const seen = new Set();
  for (let i = 0; i < 1000; i += 1) {
    const code = generateShortCode();
    assert.match(code, /^[A-Za-z0-9]{7}$/);
    seen.add(code);
  }
  assert.ok(seen.size > 990, `unexpected collision rate: ${seen.size}/1000 unique`);
});

test('a QH8Z-owned alias is rejected before any upstream Shlink request', async () => {
  const originalQuery = pool.query;
  const originalFetch = globalThis.fetch;
  const calls = [];
  let fetched = false;
  pool.query = async (sql, params = []) => {
    const text = String(sql).trim();
    calls.push({ sql: text, params });
    if (text.startsWith('INSERT INTO shlink_create_intents')) return { rows: [{ short_code: 'owned-alias' }] };
    if (text.startsWith('SELECT id FROM links')) return { rows: [{ id: 'existing-link' }] };
    if (text.startsWith('DELETE FROM shlink_create_intents')) return { rows: [] };
    throw new Error(`Unexpected query: ${text}`);
  };
  globalThis.fetch = async () => { fetched = true; throw new Error('Shlink should not be contacted'); };

  try {
    await assert.rejects(
      createShortUrl({ longUrl: 'https://example.com/new', customSlug: 'owned-alias', title: null }),
      error => error?.status === 409 && /already exists/i.test(error.message)
    );
    assert.equal(fetched, false);
    assert.ok(calls.some(call => call.sql.startsWith('SELECT id FROM links')));
    assert.ok(calls.some(call => call.sql.startsWith('DELETE FROM shlink_create_intents')));
  } finally {
    pool.query = originalQuery;
    globalThis.fetch = originalFetch;
  }
});
