import test from 'node:test';
import assert from 'node:assert/strict';
import { pool } from '../src/db.mjs';
import { consumeAuthToken, createAuthToken } from '../src/tokens.mjs';

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

test('stale reset token consumption can be recovered and re-consumed atomically', async () => {
  const originalConnect = pool.connect;
  const calls = [];
  let released = false;
  pool.connect = async () => ({
    query: async (sql, params = []) => {
      const text = String(sql).trim();
      calls.push({ sql: text, params });
      if (text.startsWith('SELECT user_id,used_at')) return { rows: [{ user_id: 'user-1', used_at: new Date(Date.now() - 3 * 60_000).toISOString(), expires_at: new Date(Date.now() + 30 * 60_000).toISOString() }] };
      if (text.includes('RETURNING user_id')) return { rows: [{ user_id: 'user-1' }] };
      return { rows: [] };
    },
    release: () => { released = true; },
  });
  try {
    const userId = await consumeAuthToken('token-1', 'reset_password');
    assert.equal(userId, 'user-1');
    assert.ok(calls.some(call => call.sql.startsWith('UPDATE auth_tokens SET used_at=NULL')));
    assert.ok(calls.some(call => call.sql.startsWith('UPDATE auth_tokens SET used_at=NOW()')));
    assert.equal(calls.at(-1).sql, 'COMMIT');
    assert.equal(released, true);
  } finally {
    pool.connect = originalConnect;
  }
});

test('MFA challenges are never revived after consumption', async () => {
  const originalConnect = pool.connect;
  const calls = [];
  pool.connect = async () => ({
    query: async (sql, params = []) => {
      const text = String(sql).trim();
      calls.push({ sql: text, params });
      if (text.startsWith('SELECT user_id,used_at')) return { rows: [{ user_id: 'user-1', used_at: new Date(Date.now() - 30 * 60_000).toISOString(), expires_at: new Date(Date.now() + 30 * 60_000).toISOString() }] };
      return { rows: [] };
    },
    release: () => {},
  });
  try {
    const userId = await consumeAuthToken('token-1', 'mfa_login');
    assert.equal(userId, null);
    assert.equal(calls.at(-1).sql, 'ROLLBACK');
    assert.equal(calls.some(call => call.sql.startsWith('UPDATE auth_tokens SET used_at=NULL')), false);
  } finally {
    pool.connect = originalConnect;
  }
});
