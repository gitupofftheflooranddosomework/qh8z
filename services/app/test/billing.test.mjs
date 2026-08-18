import test from 'node:test';
import assert from 'node:assert/strict';
import {
  applySubscription, cancelBillingForUser, cancelSubscriptionsForCustomer, checkoutIdempotencyKey, createCheckout, createPortal,
  subscriptionPriceIds, subscriptionQualifiesForPro,
} from '../src/billing.mjs';

function activeSubscription(overrides = {}) {
  return {
    id: 'sub_1',
    status: 'active',
    customer: 'cus_1',
    metadata: { user_id: 'user-1' },
    items: { data: [{ price: { id: 'price_pro' } }] },
    ...overrides,
  };
}

function entitlementClient({ user = { id: 'user-1', plan: 'free', stripe_customer_id: null }, existingSubscription = null, entitlement = true } = {}) {
  const calls = [];
  const client = {
    calls,
    async query(sql, params = []) {
      calls.push({ sql, params });
      if (sql.startsWith('SELECT pg_advisory_xact_lock')) return { rows: [{}] };
      if (sql.startsWith('SELECT id,plan,stripe_customer_id FROM users WHERE id=')) return { rows: user ? [user] : [] };
      if (sql.startsWith('SELECT id,plan,stripe_customer_id FROM users WHERE stripe_customer_id=')) return { rows: user ? [user] : [] };
      if (sql.startsWith('SELECT id FROM users WHERE stripe_customer_id=')) return { rows: [] };
      if (sql.startsWith('SELECT user_id,event_created_at FROM stripe_subscriptions')) return { rows: existingSubscription ? [existingSubscription] : [] };
      if (sql.startsWith('INSERT INTO stripe_subscriptions')) return { rows: [] };
      if (sql.startsWith('SELECT EXISTS(SELECT 1 FROM stripe_subscriptions')) return { rows: [{ pro: entitlement }] };
      if (sql.startsWith('UPDATE users SET plan=')) return { rows: [] };
      if (sql.startsWith('INSERT INTO audit_events')) return { rows: [] };
      throw new Error(`Unexpected query: ${sql}`);
    },
  };
  return client;
}

test('subscription price IDs are normalized from expanded and string prices', () => {
  assert.deepEqual(subscriptionPriceIds({ items: { data: [
    { price: { id: 'price_pro' } },
    { price: 'price_addon' },
    { price: { id: 'price_pro' } },
  ] } }), ['price_pro', 'price_addon']);
});

test('Pro entitlement requires both an eligible status and the configured Pro price', () => {
  assert.equal(subscriptionQualifiesForPro(activeSubscription(), 'price_pro'), true);
  assert.equal(subscriptionQualifiesForPro(activeSubscription(), 'price_other'), false);
  assert.equal(subscriptionQualifiesForPro(activeSubscription({ status: 'canceled' }), 'price_pro'), false);
  assert.equal(subscriptionQualifiesForPro(activeSubscription({ status: 'past_due' }), 'price_pro'), true);
  assert.equal(subscriptionQualifiesForPro(activeSubscription(), ''), false);
});

test('qualifying subscription persists state and activates Pro', async () => {
  const client = entitlementClient();
  const result = await applySubscription(client, activeSubscription(), { eventCreated: 100, proPriceId: 'price_pro' });

  assert.deepEqual(result, { applied: true, plan: 'pro', qualifiesPro: true });
  const persisted = client.calls.find(call => call.sql.startsWith('INSERT INTO stripe_subscriptions'));
  assert.ok(persisted);
  assert.equal(persisted.params[0], 'sub_1');
  assert.equal(persisted.params[3], 'active');
  assert.equal(persisted.params[5], true);
  assert.equal(persisted.params[6], 100);
  const planUpdate = client.calls.find(call => call.sql.startsWith('UPDATE users SET plan='));
  assert.deepEqual(planUpdate.params, ['pro', 'cus_1', 'user-1']);
  const audit = client.calls.find(call => call.sql.startsWith('INSERT INTO audit_events'));
  assert.equal(audit.params[1], 'billing.pro_active');
});

test('canceling one subscription does not downgrade while another still qualifies', async () => {
  const client = entitlementClient({
    user: { id: 'user-1', plan: 'pro', stripe_customer_id: 'cus_1' },
    existingSubscription: { user_id: 'user-1', event_created_at: 100 },
    entitlement: true,
  });
  const canceled = activeSubscription({ status: 'canceled' });
  const result = await applySubscription(client, canceled, { eventCreated: 200, proPriceId: 'price_pro' });

  assert.deepEqual(result, { applied: true, plan: 'pro', qualifiesPro: false });
  const planUpdate = client.calls.find(call => call.sql.startsWith('UPDATE users SET plan='));
  assert.equal(planUpdate.params[0], 'pro');
  const audit = client.calls.find(call => call.sql.startsWith('INSERT INTO audit_events'));
  assert.equal(audit.params[1], 'billing.subscription_synced');
});

test('older Stripe events are ignored without rewriting subscription or plan state', async () => {
  const client = entitlementClient({
    user: { id: 'user-1', plan: 'pro', stripe_customer_id: 'cus_1' },
    existingSubscription: { user_id: 'user-1', event_created_at: 300 },
    entitlement: true,
  });
  const result = await applySubscription(client, activeSubscription({ status: 'canceled' }), { eventCreated: 200, proPriceId: 'price_pro' });

  assert.deepEqual(result, { applied: false, reason: 'stale_event' });
  assert.equal(client.calls.some(call => call.sql.startsWith('INSERT INTO stripe_subscriptions')), false);
  assert.equal(client.calls.some(call => call.sql.startsWith('UPDATE users SET plan=')), false);
  const audit = client.calls.find(call => call.sql.startsWith('INSERT INTO audit_events'));
  assert.equal(audit.params[1], 'billing.subscription_stale_ignored');
});

test('wrong-price active subscription is persisted but cannot grant Pro', async () => {
  const client = entitlementClient({ entitlement: false });
  const result = await applySubscription(client, activeSubscription(), { eventCreated: 100, proPriceId: 'price_other' });
  assert.deepEqual(result, { applied: true, plan: 'free', qualifiesPro: false });
  const persisted = client.calls.find(call => call.sql.startsWith('INSERT INTO stripe_subscriptions'));
  assert.equal(persisted.params[5], false);
});

test('subscription events for deleted users are audited without FK actors', async () => {
  const calls = [];
  const client = {
    async query(sql, params = []) {
      calls.push({ sql, params });
      if (sql.startsWith('SELECT pg_advisory_xact_lock')) return { rows: [{}] };
      if (sql.startsWith('SELECT id,plan,stripe_customer_id FROM users WHERE id=')) return { rows: [] };
      if (sql.startsWith('SELECT id,plan,stripe_customer_id FROM users WHERE stripe_customer_id=')) return { rows: [] };
      if (sql.startsWith('INSERT INTO audit_events')) return { rows: [] };
      throw new Error(`Unexpected query: ${sql}`);
    }
  };

  const result = await applySubscription(client, activeSubscription({
    id: 'sub_deleted_user',
    customer: 'cus_deleted_user',
    metadata: { user_id: 'user-that-no-longer-exists' },
  }), { eventCreated: 100, proPriceId: 'price_pro' });

  assert.deepEqual(result, { applied: false, reason: 'user_missing' });
  const audit = calls.find(call => call.sql.startsWith('INSERT INTO audit_events'));
  assert.equal(audit.params[0], null);
  assert.equal(audit.params[1], 'billing.subscription_user_missing');
});

test('already-Pro accounts cannot create another checkout subscription', async () => {
  await assert.rejects(
    createCheckout({ id: 'user-1', plan: 'pro', email: 'user@example.com' }),
    error => error?.status === 409 && error?.code === 'already_pro'
  );
});

test('unconfigured billing returns controlled service errors', async () => {
  await assert.rejects(
    createCheckout({ id: 'user-1', plan: 'free', email: 'user@example.com' }),
    error => error?.status === 503 && error?.code === 'billing_unavailable'
  );
  await assert.rejects(
    createPortal({ id: 'user-1', plan: 'free', email: 'user@example.com' }),
    error => error?.status === 503 && error?.code === 'billing_unavailable'
  );
});

test('billed account deletion fails closed if Stripe is unavailable', async () => {
  assert.equal(await cancelBillingForUser({ id: 'user-1', stripe_customer_id: null }), 0);
  await assert.rejects(
    cancelBillingForUser({ id: 'user-1', stripe_customer_id: 'cus_1' }),
    error => error?.status === 503 && error?.code === 'billing_unavailable'
  );
});

test('checkout creation uses a stable per-user ten-minute idempotency bucket', () => {
  const bucketStart = 1_200_000;
  const a = checkoutIdempotencyKey('user-1', bucketStart);
  const b = checkoutIdempotencyKey('user-1', bucketStart + 5 * 60_000);
  const c = checkoutIdempotencyKey('user-1', bucketStart + 11 * 60_000);
  const other = checkoutIdempotencyKey('user-2', bucketStart);
  assert.equal(a, b);
  assert.notEqual(a, c);
  assert.notEqual(a, other);
});

test('account billing cancellation auto-paginates and skips terminal subscriptions', async () => {
  const canceled = [];
  const subscriptions = [
    { id: 'sub_active_1', status: 'active' },
    { id: 'sub_canceled', status: 'canceled' },
    { id: 'sub_expired', status: 'incomplete_expired' },
    { id: 'sub_active_2', status: 'past_due' },
  ];
  const stripeClient = {
    subscriptions: {
      list(params) {
        assert.deepEqual(params, { customer: 'cus_1', status: 'all', limit: 100 });
        return {
          async *[Symbol.asyncIterator]() {
            for (const subscription of subscriptions) yield subscription;
          }
        };
      },
      async cancel(id) { canceled.push(id); },
    }
  };

  const count = await cancelSubscriptionsForCustomer(stripeClient, 'cus_1');
  assert.equal(count, 2);
  assert.deepEqual(canceled, ['sub_active_1', 'sub_active_2']);
});
