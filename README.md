# QH8Z

**Short links. Long lifespan.**

QH8Z is a commercial link-management product for `qh8z.com`. It combines the strongest parts of two mature open-source approaches without running two competing redirect stacks:

- **Shlink v5.1.5** is the isolated, replaceable redirect and visit-tracking engine.
- **Kutt v3.2.6** is an MIT-licensed product/UX donor and reference.
- **QH8Z-owned code** provides identity, email verification/recovery, plans, billing, link ownership, QR delivery, destination policy, abuse controls, moderation, and the complete customer-facing brand.

## Public-launch feature set

- QH8Z marketing site and responsive dashboard
- account registration/login/logout, verified email, password recovery/change, data export and deletion
- one-time hashed verification/reset tokens; recovery tokens stay in URL fragments
- TOTP two-factor authentication with encrypted secrets and hashed one-time recovery codes; required for public administrators
- Secure HttpOnly production sessions and cookie-authenticated same-origin protection
- Cloudflare Turnstile on public auth/recovery/abuse forms
- authenticated short-link creation and custom aliases
- reserved-route and local/private-network destination protection
- Google Web Risk checks on creation and edits, plus recurring active-link rechecks with fail-closed public mode
- editable destinations, redirect disabling and account suspension
- visit analytics with anonymized stored visitor addresses and per-link QR codes
- Free/Pro plan limits and optional Stripe subscriptions
- abuse reporting, moderation queue, admin user controls and audit events
- PostgreSQL with isolated QH8Z/Shlink databases
- Caddy automatic HTTPS, security headers and access logging
- readiness/liveness endpoints, backup/restore scripts and production pre/post-deploy checks
- CI, dependency audit, Dependabot and full Docker end-to-end smoke tests
- MIT notices/provenance for upstream code

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

Redirects bypass the Node product layer: visitors go Caddy -> Shlink -> destination. Customer/business operations go through QH8Z, which talks to Shlink only over its documented REST API.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Local development

```bash
cp .env.example .env
openssl rand -hex 32
docker compose up --build
```

Open the product at `http://localhost:3000` and local redirects at `http://localhost:8080`.

## Public deployment

```bash
cp .env.production.example .env
# fill real secrets and service credentials
bash scripts/preflight.sh .env
docker compose --env-file .env --profile production up -d --build
bash scripts/bootstrap-admin.sh .env
# sign in and enroll administrator MFA
bash scripts/postdeploy.sh https://qh8z.com
```

`PUBLIC_LAUNCH_MODE=true` deliberately refuses a healthy launch if critical protections such as HTTPS/secure cookies, verified-email mode, Web Risk, Turnstile, SMTP, legal operator metadata, or administrator MFA are missing.

Read [`docs/LAUNCH.md`](docs/LAUNCH.md) before opening signup publicly.

## Monetization

Stripe is optional until paid plans are enabled. Stripe configuration is intentionally all-or-none.

| Plan | Active links | Listed price |
|---|---:|---:|
| Free | 25 | $0 |
| Pro | 5,000 | $6/month |

## Security and abuse

QH8Z does not support anonymous link creation. See [`SECURITY.md`](SECURITY.md), the public `/security` page, and `/.well-known/security.txt`.

## Open-source provenance

Shlink and Kutt are MIT-licensed upstream projects. QH8Z preserves relevant license texts in [`licenses/`](licenses/) and provenance in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md). QH8Z's original product code does not currently declare a project-wide open-source license.
