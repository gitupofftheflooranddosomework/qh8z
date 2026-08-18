# qh8z commercial product

This document describes the launch-critical commercial behavior implemented for Issue #3 Gate 4. Pricing is intentionally not finalized here; final customer-facing prices remain a Gate 6 launch decision.

## Plans and enforced limits

| Capability | Free | Pro |
| --- | ---: | ---: |
| Stored links | 100 | 10,000 |
| Custom domains | 0 | 10 |
| Analytics lookback | 7 days | 90 days |
| QR codes | Included | Included |

The API reads the workspace billing state before enforcing entitlements. A canceled Pro subscription falls back to Free entitlements. A past-due subscription currently retains Pro entitlements as a grace state; this can be tightened when final billing policy is approved.

Usage is computed from durable PostgreSQL state and is exposed at `GET /api/v1/usage` together with the active plan limits.

## Link management

Authenticated workspace members can list links. Link writers can create, edit, enable/disable, and delete links through the versioned API. Destination edits are passed through the same destination validation, managed URL rules, and reputation checks used during initial creation.

Disabled and suspended links are removed from the redirect path. Workspace analytics include total links, active links, total visits, period visits, daily traffic, top links, and referrers.

The browser dashboard is available at `/dashboard` and provides link creation and management, QR access, analytics, usage, domains, and billing actions.

## Custom domains

Custom domains are Pro-entitled workspace resources.

When a domain is created qh8z returns a DNS verification instruction:

```text
Type:  TXT
Name:  _qh8z.<custom-domain>
Value: qh8z-verification=<random-token>
```

`POST /api/v1/domains/{id}/verify` resolves that TXT record and marks the domain verified only after an exact token match.

A branded short link resolves only when the HTTP Host matches its verified custom domain. The same branded link is not exposed through the primary qh8z hostname.

Production TLS and DNS routing for verified custom domains are handled by the Gate 5 deployment layer; application-level verification does not by itself provision a certificate.

## QR codes

`GET /api/v1/links/{slug}/qr.png` returns a PNG QR code containing the effective short URL, including the custom hostname when one is assigned.

## Stripe billing

Production requires:

```text
QH8Z_BILLING_MODE=stripe
STRIPE_SECRET_KEY=...
STRIPE_WEBHOOK_SECRET=...
STRIPE_PRO_PRICE_ID=...
```

Development defaults to `QH8Z_BILLING_MODE=disabled`.

The application creates hosted subscription Checkout Sessions and Stripe customer-portal Sessions. Billing actions require an owner/admin browser session rather than an API key.

The webhook endpoint is:

```text
POST /api/v1/billing/webhook
```

It verifies the Stripe signature against the raw request body and accepts a five-minute timestamp tolerance. Webhook event IDs are durably claimed in PostgreSQL to make processing idempotent; a failed event releases its claim so Stripe can retry it.

The commercial state currently handles:

- `checkout.session.completed`
- `customer.subscription.created`
- `customer.subscription.updated`
- `customer.subscription.deleted`

Workspace IDs are carried in Checkout and subscription metadata. Subscription status is mapped to qh8z `active`, `past_due`, or `canceled` state.

The webhook route bypasses qh8z's normal customer API rate-limit bucket because it is provider-to-provider traffic, but it still requires a valid Stripe signature before processing.

## Production requirements still outside Gate 4

The commercial application layer is not the final launch deployment. Issue #3 Gate 5 must still provide the production target, TLS and domain routing, secrets management, observability, automated backups, rollback procedures, and load/failure validation. Gate 6 still owns final pricing, legal policies, end-to-end production payment testing, security review, and the qh8z.com smoke test.
