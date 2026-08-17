# QH8Z launch runbook

The codebase is intended to be deployable as a controlled/private beta without application-code changes. Public launch still requires real infrastructure, secrets, operations, and legal review.

## 1. DNS and server

Provision a Linux host with Docker/Compose and persistent storage. Point `qh8z.com` and `www.qh8z.com` at it. Allow inbound TCP 80/443 and UDP 443 if HTTP/3 is desired. Do not expose PostgreSQL publicly. QH8Z's app and Shlink development ports bind to `127.0.0.1`; Caddy is the public entry point.

## 2. Production environment

```bash
cp .env.production.example .env
```

Generate three independent secrets:

```bash
openssl rand -hex 32  # POSTGRES_PASSWORD
openssl rand -hex 32  # SHLINK_API_KEY
openssl rand -hex 32  # ADMIN_BOOTSTRAP_SECRET
```

Set the real `ADMIN_EMAIL`, `SUPPORT_EMAIL`, and `WEB_RISK_API_KEY`. Keep `WEB_RISK_REQUIRED=true`, `COOKIE_SECURE=true`, and all HTTPS URLs from the production template.

## 3. Start and verify

```bash
docker compose --profile production pull
docker compose --profile production up -d --build
curl -fsS https://qh8z.com/healthz
```

The health response should report the product database healthy, Shlink configured, and URL reputation checking configured/required.

## 4. Bootstrap the administrator

The email in `ADMIN_EMAIL` is reserved and cannot be claimed through the normal browser signup form. Create it once using the bootstrap secret:

```bash
curl -fsS -c admin.cookies \
  -H 'content-type: application/json' \
  -H "x-qh8z-admin-bootstrap: $ADMIN_BOOTSTRAP_SECRET" \
  -d '{"name":"QH8Z Admin","email":"REPLACE_WITH_ADMIN_EMAIL","password":"REPLACE_WITH_A_LONG_UNIQUE_PASSWORD"}' \
  https://qh8z.com/api/auth/register
```

Confirm the account sees the Abuse queue, then rotate or remove `ADMIN_BOOTSTRAP_SECRET` from the production environment and recreate the app container.

## 5. Product smoke test

Register a non-admin account; login/logout; create generated and custom links; verify redirects; verify visit counts; edit a destination without changing the short URL; scan a QR code; submit and moderate an abuse report; export account data; change a password; delete a disposable test account.

GitHub's integration workflow automates the core stack version of this flow against Postgres + Shlink + QH8Z.

## 6. Backups

Create a full PostgreSQL backup (both the QH8Z and Shlink databases):

```bash
./scripts/backup.sh /secure/off-host-staging-directory
```

Move encrypted copies off the application host. Periodically test restore on a disposable environment. `scripts/restore.sh` requires `CONFIRM_RESTORE=YES` because it is destructive.

## 7. Stripe (optional for initial free beta)

Create a recurring Pro price matching the displayed price. Set `STRIPE_SECRET_KEY`, `STRIPE_PRO_PRICE_ID`, and a webhook at `https://qh8z.com/api/billing/webhook` for `checkout.session.completed` and `customer.subscription.deleted`; set `STRIPE_WEBHOOK_SECRET`; test checkout, Billing Portal, cancellation, and plan downgrade.

## 8. Before broad open signup

- Operate monitored `support@` / abuse / security mailboxes.
- Configure off-host encrypted backup retention and perform restore drills.
- Add uptime, error, disk, database, and certificate monitoring/alerting.
- Define log-retention/privacy policy.
- Obtain jurisdiction-specific review of the Privacy and Terms launch drafts.
- Configure tax/accounting requirements before charging customers.
- Own dependency/security update triage and stage Shlink upgrades before production.
- Decide whether to keep Google Web Risk Lookup or move to a higher-throughput reputation design as traffic grows.
