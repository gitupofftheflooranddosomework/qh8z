import Stripe from 'stripe';
import { config } from './config.mjs';
import { pool, audit } from './db.mjs';

const stripe = config.stripeSecretKey ? new Stripe(config.stripeSecretKey) : null;

export function billingEnabled() {
  return Boolean(stripe && config.stripeProPriceId);
}

export async function createCheckout(user) {
  if (!billingEnabled()) throw new Error('Billing is not configured');
  return stripe.checkout.sessions.create({
    mode: 'subscription',
    customer: user.stripe_customer_id || undefined,
    customer_email: user.stripe_customer_id ? undefined : user.email,
    line_items: [{ price: config.stripeProPriceId, quantity: 1 }],
    success_url: `${config.appBaseUrl}/app?billing=success`,
    cancel_url: `${config.appBaseUrl}/app?billing=cancelled`,
    metadata: { user_id: user.id },
    subscription_data: { metadata: { user_id: user.id } },
    allow_promotion_codes: true
  });
}

export async function createPortal(user) {
  if (!stripe || !user.stripe_customer_id) throw new Error('No Stripe customer is attached to this account');
  return stripe.billingPortal.sessions.create({ customer: user.stripe_customer_id, return_url: `${config.appBaseUrl}/app` });
}

export async function handleStripeWebhook(rawBody, signature) {
  if (!stripe || !config.stripeWebhookSecret) throw new Error('Stripe webhook is not configured');
  const event = stripe.webhooks.constructEvent(rawBody, signature, config.stripeWebhookSecret);

  if (event.type === 'checkout.session.completed') {
    const session = event.data.object;
    const userId = session.metadata?.user_id;
    if (userId) {
      await pool.query('UPDATE users SET plan=$1,stripe_customer_id=COALESCE($2,stripe_customer_id) WHERE id=$3', ['pro', session.customer || null, userId]);
      await audit(userId, 'billing.pro_activated', userId, { stripeSessionId: session.id });
    }
  }

  if (event.type === 'customer.subscription.deleted') {
    const subscription = event.data.object;
    const userId = subscription.metadata?.user_id;
    if (userId) {
      await pool.query('UPDATE users SET plan=$1 WHERE id=$2', ['free', userId]);
      await audit(userId, 'billing.pro_cancelled', userId, { subscriptionId: subscription.id });
    }
  }

  return event.type;
}
