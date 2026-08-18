import test from 'node:test';
import assert from 'node:assert/strict';
import { applySubscription, createCheckout } from '../src/billing.mjs';

test('subscription events for deleted users are committed as orphan audits without FK actors', async () => {
  const calls = [];
  const client = {
    async query(sql, params) {
      calls.push({ sql, params });
      if (sql.startsWith('UPDATE users')) return { rows: [] };
      if (sql.startsWith('INSERT INTO audit_events')) return { rows: [] };
      throw new Error(`Unexpected query: ${sql}`);
    }
  };

  const result = await applySubscription(client, {
    id: 'sub_deleted_user',
    status: 'canceled',
    customer: 'cus_deleted_user',
    metadata: { user_id: 'user-that-no-longer-exists' },
  });

  assert.deepEqual(result, { applied: false, reason: 'user_missing' });
  assert.equal(calls.length, 2);
  assert.equal(calls[1].params[0], null);
  assert.equal(calls[1].params[1], 'billing.subscription_user_missing');
  assert.equal(calls[1].params[2], 'user-that-no-longer-exists');
});

test('subscription events for existing users keep the user as the audit actor', async () => {
  const calls = [];
  const client = {
    async query(sql, params) {
      calls.push({ sql, params });
      if (sql.startsWith('UPDATE users')) return { rows: [{ id: 'user-1' }] };
      if (sql.startsWith('INSERT INTO audit_events')) return { rows: [] };
      throw new Error(`Unexpected query: ${sql}`);
    }
  };

  const result = await applySubscription(client, {
    id: 'sub_active',
    status: 'active',
    customer: 'cus_1',
    metadata: { user_id: 'user-1' },
  });

  assert.deepEqual(result, { applied: true, plan: 'pro' });
  assert.equal(calls[1].params[0], 'user-1');
  assert.equal(calls[1].params[1], 'billing.pro_active');
});

test('already-Pro accounts cannot create another checkout subscription', async () => {
  await assert.rejects(
    createCheckout({ id: 'user-1', plan: 'pro', email: 'user@example.com' }),
    error => error?.status === 409 && error?.code === 'already_pro'
  );
});
