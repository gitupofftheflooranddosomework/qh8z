# QH8Z

**Short links. Long lifespan.**

QH8Z is a commercial link-management product for `qh8z.com`. It combines the strongest parts of two mature open-source approaches without running two competing redirect stacks:

- **Shlink v5.1.5** is the isolated, replaceable redirect and visit-tracking engine.
- **Kutt v3.2.6** is an MIT-licensed product/UX donor and reference.
- **QH8Z-owned code** provides the customer product, identity, billing, link ownership, developer API, safety policy, moderation, recovery tooling, and brand.

## Launch-v1 product

### Link workspace

- authenticated short-link creation with generated or custom aliases
- searchable, paginated link library with status and tag filtering
- titles, tags, and private notes
- editable destinations while keeping the same short URL
- archive/unarchive without breaking the redirect
- disable and restore to the same owned short code
- per-link QR codes
- CSV inventory export and full account JSON export
- visit totals, human/bot summaries, recent visit history, and dashboard analytics

### Pro controls

- 5,000 active links instead of 25
- scheduled link expiration
- maximum-visit limits
- bulk creation with per-row results
- scoped, revocable developer API tokens
- versioned bearer-token API under `/api/v1`
- optional Stripe Checkout and self-service billing portal

See [`docs/API.md`](docs/API.md) for the developer surface.

### Accounts and safety

- registration/login/logout, verified email, password recovery/change, account export and deletion
- one-time hashed verification/reset tokens; recovery credentials remain out of ordinary server URLs
- TOTP two-factor authentication with encrypted secrets and hashed single-use recovery codes
- administrator MFA required in public-launch mode
- Secure HttpOnly sessions and cookie-authenticated same-origin protection
- Cloudflare Turnstile on public auth/recovery/abuse forms
- Google Web Risk checks on creation/edits plus recurring active-link rechecks
- literal and DNS-resolved local/private/reserved destination protection
- durable QH8Z↔Shlink ownership/reconciliation and orphan-redirect cleanup
- abuse reporting, moderation queue, link disable, user suspension, and audit events

### Operations

- PostgreSQL with isolated least-privilege QH8Z/Shlink roles and databases
- Caddy automatic HTTPS/security headers and blocked public Shlink management namespace
- Shlink visit-IP anonymization and no per-request edge access log in the shipped Caddy config
- readiness/liveness probes and bounded dependency timeouts
- graceful HTTP/background-worker shutdown
- deterministic npm lockfile and dependency audit
- two-database backup/restore with checksum validation and destructive restore CI drill
- guarded deploy/rollback scripts and protected manual production workflow
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

Redirects bypass the Node product layer: visitors go Caddy → Shlink → destination. Customer/business operations go through QH8Z, which talks to Shlink over its REST API. Shlink's `/rest` management namespace is blocked at the public edge.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Local development

```bash
cp .env.example .env
openssl rand -hex 32
docker compose up --build
```

Open the product at `http://localhost:3000` and local redirects at `http://localhost:8080`.

## Automated acceptance gates

QH8Z has three separate launch gates:

1. **CI** — deterministic install, syntax, unit/security/product-rule tests, npm audit, and operator-script syntax.
2. **Integration** — full PostgreSQL + Shlink + QH8Z + Caddy HTTPS behavior, authentication/MFA/recovery/moderation, namespace isolation, database-role isolation, and destructive backup/restore.
3. **Product acceptance** — real Free→Pro customer flow including advanced link controls, search/filtering, archive/restore, bulk creation, CSV export, bearer API usage, and API-token revocation.

A PR head is not considered launch-ready until all three pass on that exact head.

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

`PUBLIC_LAUNCH_MODE=true` deliberately refuses a healthy launch if critical protections such as HTTPS/secure cookies, verified-email mode, Web Risk, Turnstile, SMTP, legal operator metadata, or administrator MFA are missing. New signup can still be closed independently with `ALLOW_SIGNUP=false` without disabling the rest of the public security posture.

Read [`docs/LAUNCH.md`](docs/LAUNCH.md) before opening signup publicly.

## Pricing

Stripe is optional until paid plans are enabled. Stripe configuration is intentionally all-or-none.

| Plan | Active links | Included | Listed price |
|---|---:|---|---:|
| Free | 25 | aliases, tags/notes, QR, analytics, search, archive/restore, CSV export | $0 |
| Pro | 5,000 | everything in Free + expiration, visit limits, bulk creation, developer API | $6/month |

## Security and abuse

QH8Z does not support anonymous link creation. See [`SECURITY.md`](SECURITY.md), the public `/security` page, and `/.well-known/security.txt`.

## Open-source provenance

Shlink and Kutt are MIT-licensed upstream projects. QH8Z preserves relevant license texts in [`licenses/`](licenses/) and provenance in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md). QH8Z's original product code does not currently declare a project-wide open-source license.
