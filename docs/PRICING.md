# qh8z launch pricing and limits

**Launch pricing decision — August 18, 2026**

All customer-facing qh8z prices are denominated in Canadian dollars.

## Free — C$0

- up to 100 stored links per workspace
- qh8z primary short domain
- generated or custom slugs
- QR codes
- 7-day analytics lookback
- link editing, disabling, and deletion
- API access subject to normal service rate limits
- no custom domains

## Pro — C$12.00 per month

- up to 10,000 stored links per workspace
- up to 10 verified custom domains
- generated or custom slugs
- QR codes
- 90-day analytics lookback
- link editing, disabling, and deletion
- API access subject to normal service rate limits
- Stripe-hosted subscription management

Applicable taxes may be added where required.

## Billing policy at launch

- Pro is a monthly recurring subscription.
- There is no annual plan at initial launch; do not advertise an annual discount until the billing implementation supports a separate annual Stripe price.
- Cancellation is managed through the Stripe customer portal and takes effect according to the subscription state returned by Stripe.
- A canceled workspace falls back to Free entitlements.
- A past-due workspace temporarily retains Pro entitlements as the launch grace behavior while payment recovery is in progress.
- qh8z does not silently allow resource creation above the plan's enforced limits.

## Product enforcement

These limits match the limits enforced by the qh8z application. A pricing-page change that changes an entitlement must be accompanied by the corresponding application/test change so published limits and enforcement cannot drift.

The production Stripe product must have a recurring Pro price of exactly **C$12.00/month** and its price identifier must be configured as `STRIPE_PRO_PRICE_ID` before the billing end-to-end launch test can pass.

## Future pricing changes

Future plans, annual billing, introductory discounts, higher-volume tiers, or enterprise pricing are post-launch decisions. They must not be shown as available until checkout and entitlement enforcement support them.