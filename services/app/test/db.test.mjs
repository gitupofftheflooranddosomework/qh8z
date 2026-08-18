import test from 'node:test';
import assert from 'node:assert/strict';
import { audit, pool } from '../src/db.mjs';

test('audit write failures do not throw into completed business operations', async () => {
  const originalQuery = pool.query;
  const originalError = console.error;
  const logs = [];
  pool.query = async () => { throw new Error('simulated audit storage failure'); };
  console.error = message => logs.push(String(message));
  try {
    assert.equal(await audit('user-1', 'test.event', 'target-1'), false);
    assert.ok(logs.some(line => line.includes('audit.write_failed')));
  } finally {
    pool.query = originalQuery;
    console.error = originalError;
  }
});
