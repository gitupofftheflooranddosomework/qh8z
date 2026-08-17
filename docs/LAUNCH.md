# QH8Z launch runbook

The codebase is deployable as a controlled/private beta without code changes. Public launch still requires real infrastructure, secrets, operations and legal review.

## DNS and server

Provision a Linux host with Docker/Compose and persistent storage. Point `qh8z.com` and `www.qh8z.com` at it. Allow inbound 80/443 (and UDP 443 for HTTP/3). Do not expose PostgreSQL publicly. In production, firewall development ports 3000/8080 so Caddy is the public entry point.

## Secrets

Copy `.env.example` to `.env`. Set strong unique `POSTGRES_PASSWORD` and `SHLINK_API_KEY`, plus `ADMIN_EMAIL`. Use `NODE_ENV=production`, `APP_BASE_URL=https://qh8z.com`, `PUBLIC_SHORT_BASE_URL=https://qh8z.com`, `SHLINK_HTTPS_ENABLED=true`, and `COOKIE_SECURE=true`.

## Start

```bash
docker compose --profile production pull
docker compose --profile production up -d --build
curl -fsS https://qh8z.com/healthz
```

Create the admin account using the exact email in `ADMIN_EMAIL`.

## Smoke test

Register/login/logout; create generated and custom links; verify redirects; verify visit counts; edit a destination without changing the short URL; scan QR; submit/report/moderate an abusive link; export data; change password; delete a test account.

## Stripe (optional for initial free beta)

Create a recurring Pro price matching the displayed price. Set `STRIPE_SECRET_KEY`, `STRIPE_PRO_PRICE_ID`, and a webhook at `https://qh8z.com/api/billing/webhook` for `checkout.session.completed` and `customer.subscription.deleted`; set `STRIPE_WEBHOOK_SECRET`; test checkout, portal and cancellation.

## Before open signup at scale

Add automated malicious-destination reputation scanning, monitored support/abuse mailboxes, off-host encrypted PostgreSQL backups with restore tests, uptime/error monitoring, log-retention policy, jurisdiction-specific privacy/terms legal review, tax/account configuration for paid plans, dependency/security update ownership, and a staging environment for Shlink upgrades.
