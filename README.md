# QH8Z

**Short links. Long lifespan.**

QH8Z is a commercial link-shortening product for `qh8z.com`. It combines the strongest parts of two mature open-source approaches without running two competing redirect stacks:

- **Shlink v5.1.5** is the replaceable redirect and visit-tracking engine.
- **Kutt v3.2.6** is a product/UX feature donor and reference for the multi-user shortener experience.
- **QH8Z-owned code** provides the customer-facing brand, accounts, sessions, plans, billing, link ownership, QR delivery, destination reputation checks, abuse reporting, moderation, and product UI.

The result is intentionally a QH8Z product rather than a skin on either upstream project.

## What works

- QH8Z-branded marketing site and responsive dashboard
- account registration, login, logout, password changes, data export and deletion
- server-side sessions with HttpOnly/SameSite cookies and bcrypt password hashing
- authenticated short-link creation and custom aliases
- destination editing and redirect disabling
- Google Web Risk destination checks on creation and edits, with fail-closed production mode
- visit totals with bot/non-bot summaries plus raw per-link visit API
- per-link QR codes
- Free and Pro plan limits
- optional Stripe Checkout, Billing Portal and webhook plan activation
- public abuse reporting, admin moderation and audit events
- protected one-time admin bootstrap flow
- request rate limits, origin checks and security headers
- PostgreSQL persistence with isolated QH8Z/Shlink databases
- production Caddy reverse proxy with automatic HTTPS
- backup/restore scripts
- GitHub Actions unit/syntax CI plus a full Docker integration smoke test
- Dependabot and preserved upstream MIT notices

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
# generate unique values for POSTGRES_PASSWORD and SHLINK_API_KEY
openssl rand -hex 32
docker compose up --build
```

Open QH8Z at `http://localhost:3000`. Local short redirects are served by Shlink at `http://localhost:8080`.

## Production

Start from the production template instead of adapting the development file:

```bash
cp .env.production.example .env
```

Set strong unique `POSTGRES_PASSWORD`, `SHLINK_API_KEY`, and `ADMIN_BOOTSTRAP_SECRET`; set the real `ADMIN_EMAIL`; configure a Google Web Risk API key; then point `qh8z.com` and `www.qh8z.com` at the host and run:

```bash
docker compose --profile production up -d --build
```

The app and Shlink development ports bind only to loopback. Caddy is the intended public entry point on ports 80/443.

Read [`docs/LAUNCH.md`](docs/LAUNCH.md) for the admin bootstrap, smoke test, backups, Stripe, and public-launch checklist.

## Monetization

Stripe is optional. When `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, and `STRIPE_PRO_PRICE_ID` are configured, the dashboard exposes subscription checkout and customer billing management.

| Plan | Active links | Price shown by QH8Z |
|---|---:|---:|
| Free | 25 | $0 |
| Pro | 5,000 | $6/month |

## Security / abuse

QH8Z intentionally does **not** provide anonymous shortening. Creating a redirect requires a QH8Z account. Shlink's administrative API remains private to the product service. Production link creation is designed to fail closed when the destination reputation service is unavailable.

Before broad open signup, still establish monitored abuse/security mailboxes, off-host backup retention and restore drills, production observability, and jurisdiction-specific legal review.

See [`SECURITY.md`](SECURITY.md).

## Open-source provenance

Both Shlink and Kutt are MIT-licensed upstream projects. QH8Z preserves their license texts in [`licenses/`](licenses/) and documents provenance in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

QH8Z's original product code does **not** currently declare a project-wide open-source license.
