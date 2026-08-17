# QH8Z

**Short links. Long lifespan.**

QH8Z is a commercial link-shortening product for `qh8z.com`. It combines the strongest parts of two mature open-source approaches without running two competing redirect stacks:

- **Shlink v5.1.5** is the replaceable redirect and visit-tracking engine.
- **Kutt v3.2.6** is a product/UX feature donor and reference for the multi-user shortener experience.
- **QH8Z-owned code** provides the customer-facing brand, accounts, sessions, plans, billing, link ownership, QR delivery, abuse reporting, moderation, and product UI.

The result is intentionally a QH8Z product rather than a skin on either upstream project.

## What works

- QH8Z-branded marketing site and responsive dashboard
- account registration, login, logout, password changes, data export and deletion
- server-side sessions with HttpOnly/SameSite cookies and bcrypt password hashing
- authenticated short-link creation and custom aliases
- destination editing and redirect disabling
- visit totals with bot/non-bot summaries plus raw per-link visit API
- per-link QR codes
- Free and Pro plan limits
- optional Stripe Checkout, Billing Portal and webhook plan activation
- public abuse reporting, admin moderation and audit events
- request rate limits, origin checks and security headers
- PostgreSQL persistence with isolated QH8Z/Shlink databases
- production Caddy reverse proxy with automatic HTTPS
- GitHub Actions checks and Dependabot
- preserved upstream MIT notices

## Architecture

```text
                           qh8z.com
                              |
                            Caddy
                 +------------+------------+
                 |                         |
       product/API routes              /<shortCode>
                 |                         |
              QH8Z app                 Shlink 5.1.5
                 |                         |
                 +---------+   +-----------+
                           |   |
                         PostgreSQL
                    qh8z DB + shlink DB
```

Browser clients never receive the Shlink API key. QH8Z calls Shlink over the private container network. Redirect requests bypass the Node product app and go directly from Caddy to Shlink.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Local development

```bash
cp .env.example .env
openssl rand -hex 32
# put strong generated values in POSTGRES_PASSWORD and SHLINK_API_KEY
docker compose up --build
```

Open QH8Z at `http://localhost:3000`. Local short redirects are served by Shlink at `http://localhost:8080`.

## Production

Set `QH8Z_DOMAIN=qh8z.com`, `NODE_ENV=production`, `APP_BASE_URL=https://qh8z.com`, `PUBLIC_SHORT_BASE_URL=https://qh8z.com`, `SHLINK_HTTPS_ENABLED=true`, `COOKIE_SECURE=true`, strong database/Shlink secrets, and an `ADMIN_EMAIL`. Point DNS to the host, then run:

```bash
docker compose --profile production up -d --build
```

Read [`docs/LAUNCH.md`](docs/LAUNCH.md) before opening registration.

## Monetization

Stripe is optional. When `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, and `STRIPE_PRO_PRICE_ID` are configured, the dashboard exposes subscription checkout and customer billing management.

| Plan | Active links | Price shown by QH8Z |
|---|---:|---:|
| Free | 25 | $0 |
| Pro | 5,000 | $6/month |

## Security / abuse

QH8Z intentionally does **not** provide anonymous shortening. Creating a redirect requires a QH8Z account. Shlink's administrative API remains private to the product service.

Before scaling open signup, add a commercial malicious-destination reputation provider, monitored abuse/security mailboxes, off-host backups, observability, and jurisdiction-specific legal review.

See [`SECURITY.md`](SECURITY.md).

## Open-source provenance

Both Shlink and Kutt are MIT-licensed upstream projects. QH8Z preserves their license texts in [`licenses/`](licenses/) and documents provenance in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

QH8Z's original product code does **not** currently declare a project-wide open-source license.
