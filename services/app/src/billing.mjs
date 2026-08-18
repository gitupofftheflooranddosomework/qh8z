import Stripe from 'stripe';
import { config } from './config.mjs';
import { pool } from './db.mjs';

const stripe = config.stripeSecretKey ? new Stripe(config.stripeSecretKey, {
  timeout: 10_000,
  maxNetworkRetries: 2,
}) : null;

export function billingEnabled() {
  return Boolean(stripe && config.stripeWebhookSecret && config.stripeProPriceId);
}

function billingError(message, status, code) {
  const error = new Error(message);
  error.status = status;
  error.code = code;
  return error;
}

function stripeId(value) {
  if (typeof value === 'string') return value;
  return typeof value?.id === 'string' ? value.id : null;
}

export function checkoutIdempotencyKey(userId, now = Date.now()) {
  const bucket = Math.floor(Number(now) / (10 * 60_000));
  return `qh8z-checkout:${String(userId)}:${bucket}`;
}

export async function createCheckout(user) {
  if (user?.plan === 'pro') {
    throw billingError('This account already has Pro access. Use the billing portal to manage the subscription.', 409, 'already_pro');
  }
  if (!billingEnabled()) throw billingError('Billing is temporarily unavailable.', 503, 'billing_unavailable');
  return stripe.checkout.sessions.create({
    mode: 'subscription',
    customer: user.stripe_customer_id || undefined,
    customer_email: user.stripe_customer_id ? undefined : user.email,
    line_items: [{ price: config.stripeProPriceId, quantity: 1 }],
    success_url: `${config.appBaseUrl}/app?billing=success`,
    cancel_url: `${config.appBaseUrl}/app?billing=cancelled`,
    metadata: { user_id: user.id },
    subscription_data: { metadata: { user_id: user.id } },
    allow_promotion_codes: true,
  }, { idempotencyKey: checkoutIdempotencyKey(user.id) });
}

export async function createPortal(user) {
  if (!billingEnabled()) throw billingError('Billing is temporarily unavailable.', 503, 'billing_unavailable');
  if (!user?.stripe_customer_id) throw billingError('No billing profile exists for this account yet.', 409, 'billing_profile_missing');
  return stripe.billingPortal.sessions.create({ customer: user.stripe_customer_id, return_url: `${config.appBaseUrl}/app` });
}

function proStatus(status) {
  // past_due intentionally keeps a grace-period entitlement. Stripe will send a
  // later state transition if the subscription becomes unpaid/canceled.
  return ['active', 'trialing', 'past_due'].includes(String(status || ''));
}

export function subscriptionPriceIds(subscription) {
  return [...new Set((subscription?.items?.data || [])
    .map(item => stripeId(item?.price))
    .filter(Boolean))];
}

export function subscriptionQualifiesForPro(subscription, proPriceId = config.stripeProPriceId) {
  const expected = String(proPriceId || '').trim();
  return Boolean(expected && proStatus(subscription?.status) && subscriptionPriceIds(subscription).includes(expected));
}

async function auditBilling(client, actorUserId, eventType, targetId = null, metadata = {}) {
  await client.query(
    'INSERT INTO audit_events(actor_user_id,event_type,target_id,metadata) VALUES($1,$2,$3,$4)',
    [actorUserId, eventType, targetId, JSON.stringify(metadata)]
  );
}

async function resolveSubscriptionUser(client, subscription) {
  const metadataUserId = String(subscription?.metadata?.user_id || '').trim() || null;
  const customerId = stripeId(subscription?.customer);
  let user = null;

  if (metadataUserId) {
    const result = await client.query(
      'SELECT id,plan,stripe_customer_id FROM users WHERE id=$1 FOR UPDATE',
      [metadataUserId]
    );
    user = result.rows[0] || null;
  }
  if (!user && customerId) {
    const result = await client.query(
      'SELECT id,plan,stripe_customer_id FROM users WHERE stripe_customer_id=$1 FOR UPDATE',
      [customerId]
    );
    user = result.rows[0] || null;
  }
  return { user, metadataUserId, customerId };
}

function normalizedEventCreated(value) {
  const created = Number(value);
  return Number.isSafeInteger(created) && created >= 0 ? created : 0;
}

export async function applySubscription(client, subscription, { eventCreated = 0, proPriceId = config.stripeProPriceId } = {}) {
  const subscriptionId = stripeId(subscription);
  if (!subscriptionId) return { applied: false, reason: 'missing_subscription_id' };

  // Serialize all events for one subscription before resolving its account.
  // Different subscriptions for one account are then serialized by the user row
  // lock below, so entitlement recomputation sees a committed set of siblings.
  await client.query("SELECT pg_advisory_xact_lock(hashtext($1),hashtext('qh8z-stripe-subscription'))", [subscriptionId]);

  const { user, metadataUserId, customerId } = await resolveSubscriptionUser(client, subscription);
  const targetId = metadataUserId || customerId || subscriptionId;
  if (!user) {
    await auditBilling(client, null, 'billing.subscription_user_missing', targetId, {
      subscriptionId, customerId, status: subscription?.status || null,
    });
    return { applied: false, reason: 'user_missing' };
  }

  if (customerId && user.stripe_customer_id && user.stripe_customer_id !== customerId) {
    await auditBilling(client, user.id, 'billing.subscription_customer_mismatch', user.id, {
      subscriptionId, expectedCustomerId: user.stripe_customer_id, observedCustomerId: customerId,
    });
    return { applied: false, reason: 'customer_mismatch' };
  }
  if (customerId) {
    const owner = await client.query('SELECT id FROM users WHERE stripe_customer_id=$1 AND id<>$2 LIMIT 1', [customerId, user.id]);
    if (owner.rows[0]) {
      await auditBilling(client, user.id, 'billing.subscription_customer_conflict', user.id, {
        subscriptionId, customerId, conflictingUserId: owner.rows[0].id,
      });
      return { applied: false, reason: 'customer_conflict' };
    }
  }

  const existing = await client.query(
    'SELECT user_id,event_created_at FROM stripe_subscriptions WHERE subscription_id=$1 FOR UPDATE',
    [subscriptionId]
  );
  if (existing.rows[0]?.user_id && existing.rows[0].user_id !== user.id) {
    await auditBilling(client, user.id, 'billing.subscription_owner_mismatch', user.id, {
      subscriptionId, recordedUserId: existing.rows[0].user_id,
    });
    return { applied: false, reason: 'owner_mismatch' };
  }

  const eventTimestamp = normalizedEventCreated(eventCreated);
  if (existing.rows[0] && Number(existing.rows[0].event_created_at) > eventTimestamp) {
    await auditBilling(client, user.id, 'billing.subscription_stale_ignored', user.id, {
      subscriptionId, eventCreated: eventTimestamp, recordedEventCreated: Number(existing.rows[0].event_created_at),
    });
    return { applied: false, reason: 'stale_event' };
  }

  const priceIds = subscriptionPriceIds(subscription);
  const qualifiesPro = subscriptionQualifiesForPro(subscription, proPriceId);
  await client.query(
    `INSERT INTO stripe_subscriptions(subscription_id,user_id,customer_id,status,price_ids,qualifies_pro,event_created_at)
     VALUES($1,$2,$3,$4,$5::jsonb,$6,$7)
     ON CONFLICT (subscription_id) DO UPDATE SET
       customer_id=EXCLUDED.customer_id,
       status=EXCLUDED.status,
       price_ids=EXCLUDED.price_ids,
       qualifies_pro=EXCLUDED.qualifies_pro,
       event_created_at=EXCLUDED.event_created_at,
       updated_at=NOW()`,
    [subscriptionId, user.id, customerId, String(subscription?.status || 'unknown'), JSON.stringify(priceIds), qualifiesPro, eventTimestamp]
  );

  const entitlement = await client.query(
    'SELECT EXISTS(SELECT 1 FROM stripe_subscriptions WHERE user_id=$1 AND qualifies_pro=TRUE)::boolean AS pro',
    [user.id]
  );
  const plan = entitlement.rows[0]?.pro ? 'pro' : 'free';
  await client.query(
    'UPDATE users SET plan=$1,stripe_customer_id=COALESCE($2,stripe_customer_id) WHERE id=$3',
    [plan, customerId, user.id]
  );

  const eventType = user.plan === plan ? 'billing.subscription_synced' : (plan === 'pro' ? 'billing.pro_active' : 'billing.pro_inactive');
  await auditBilling(client, user.id, eventType, user.id, {
    subscriptionId,
    customerId,
    status: subscription?.status || null,
    priceIds,
    qualifiesPro,
    plan,
  });
  return { applied: true, plan, qualifiesPro };
}

export async function handleStripeWebhook(rawBody, signature) {
  if (!stripe || !config.stripeWebhookSecret) throw new Error('Stripe webhook is not configured');
  const event = stripe.webhooks.constructEvent(rawBody, signature, config.stripeWebhookSecret);
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const inserted = await client.query(
      'INSERT INTO stripe_events(event_id,event_type) VALUES($1,$2) ON CONFLICT (event_id) DO NOTHING RETURNING event_id',
      [event.id, event.type]
    );
    if (!inserted.rows[0]) {
      await client.query('COMMIT');
      return `${event.type}:duplicate`;
    }

    if (event.type === 'checkout.session.completed') {
      const session = event.data.object;
      const userId = String(session.metadata?.user_id || '').trim();
      const customerId = stripeId(session.customer);
      if (userId && customerId) await client.query('UPDATE users SET stripe_customer_id=$1 WHERE id=$2', [customerId, userId]);
    }
    if (event.type.startsWith('customer.subscription.')) {
      await applySubscription(client, event.data.object, { eventCreated: event.created });
    }

    await client.query('COMMIT');
    return event.type;
  } catch (error) {
    try { await client.query('ROLLBACK'); } catch {}
    throw error;
  } finally {
    client.release();
  }
}

export async function cancelSubscriptionsForCustomer(stripeClient, customerId) {
  let canceled = 0;
  for await (const subscription of stripeClient.subscriptions.list({ customer: customerId, status: 'all', limit: 100 })) {
    if (['canceled', 'incomplete_expired'].includes(subscription.status)) continue;
    await stripeClient.subscriptions.cancel(subscription.id);
    canceled += 1;
  }
  return canceled;
}

export async function cancelBillingForUser(user) {
  if (!user?.stripe_customer_id) return 0;
  if (!stripe) throw billingError('Billing teardown is temporarily unavailable. The account was not deleted.', 503, 'billing_unavailable');
  return cancelSubscriptionsForCustomer(stripe, user.stripe_customer_id);
}
