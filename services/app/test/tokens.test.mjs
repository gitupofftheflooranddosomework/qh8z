import test from 'node:test';
import assert from 'node:assert/strict';
import { pool } from '../src/db.mjs';
import { createAuthToken } from '../src/tokens.mjs';

test('auth token replacement is serialized inside one transaction', async () => {
  const originalConnect = pool.connect;
  const calls = [];
  let released = false;
  pool.connect = async () => ({
    query: async (sql, params = []) => { calls.push({ sql: String(sql), params }); return { rows: [] }; },
    release: () => { released = true; },
  });
  try {
    const token = await createAuthToken('user-1', 'reset_password', 60);
    assert.match(token, /^[A-Za-z0-9_-]+$/);
    assert.equal(calls[0].sql, 'BEGIN');
    assert.ok(calls.some(call => call.sql.includes('pg_advisory_xact_lock')));
    assert.ok(calls.some(call => call.sql.startsWith('DELETE FROM auth_tokens')));
    assert.ok(calls.some(call => call.sql.startsWith('INSERT INTO auth_tokens')));
    assert.equal(calls.at(-1).sql, 'COMMIT');
    assert.equal(released, true);
  } finally {
    pool.connect = originalConnect;
  }
});
