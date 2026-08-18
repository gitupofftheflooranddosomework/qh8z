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
  return ['active', 'trialing', 'past_due'].includes(status);
}

async function auditBilling(client, actorUserId, eventType, targetId = null, metadata = {}) {
  await client.query(
    'INSERT INTO audit_events(actor_user_id,event_type,target_id,metadata) VALUES($1,$2,$3,$4)',
    [actorUserId, eventType, targetId, JSON.stringify(metadata)]
  );
}

export async function applySubscription(client, subscription) {
  const userId = subscription.metadata?.user_id;
  if (!userId) return { applied: false, reason: 'missing_user_id' };
  const plan = proStatus(subscription.status) ? 'pro' : 'free';
  const updated = await client.query(
    'UPDATE users SET plan=$1,stripe_customer_id=COALESCE($2,stripe_customer_id) WHERE id=$3 RETURNING id',
    [plan, subscription.customer || null, userId]
  );
  if (!updated.rows[0]) {
    await auditBilling(client, null, 'billing.subscription_user_missing', userId, { subscriptionId: subscription.id, status: subscription.status });
    return { applied: false, reason: 'user_missing' };
  }
  await auditBilling(client, userId, plan === 'pro' ? 'billing.pro_active' : 'billing.pro_inactive', userId, { subscriptionId: subscription.id, status: subscription.status });
  return { applied: true, plan };
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
      const userId = session.metadata?.user_id;
      if (userId && session.customer) await client.query('UPDATE users SET stripe_customer_id=$1 WHERE id=$2', [session.customer, userId]);
    }
    if (['customer.subscription.created', 'customer.subscription.updated', 'customer.subscription.deleted'].includes(event.type)) {
      await applySubscription(client, event.data.object);
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
  if (!stripe || !user?.stripe_customer_id) return 0;
  return cancelSubscriptionsForCustomer(stripe, user.stripe_customer_id);
}
