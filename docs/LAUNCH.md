# QH8Z public launch runbook

QH8Z is designed to fail closed in `PUBLIC_LAUNCH_MODE=true` when critical public-safety services are missing. The application code can be public-launch-ready while DNS, credentials, mail reputation, and legal/business operations remain external deployment requirements.

## 1. Provision the host

Use a maintained Linux host with Docker Engine and Compose, persistent storage, automatic security updates, an allowlist firewall, and enough disk for PostgreSQL/visit growth. Expose only TCP 80/443 and UDP 443 publicly. PostgreSQL, QH8Z port 3000, and Shlink port 8080 must remain private/loopback.

Point `qh8z.com` and `www.qh8z.com` to the host. Caddy obtains and renews TLS certificates automatically.

## 2. Configure production secrets and dependencies

```bash
cp .env.production.example .env
openssl rand -hex 32   # POSTGRES_PASSWORD
openssl rand -hex 32   # SHLINK_API_KEY
openssl rand -hex 32   # ADMIN_BOOTSTRAP_SECRET
openssl rand -hex 32   # MFA_ENCRYPTION_KEY
```

Fill every required field in `.env`:

- Google Web Risk API key, with `WEB_RISK_REQUIRED=true`.
- Cloudflare Turnstile site/secret keys, with `TURNSTILE_REQUIRED=true`.
- SMTP host/credentials and a domain-authenticated `MAIL_FROM` address.
- monitored `SUPPORT_EMAIL`, `ABUSE_EMAIL`, and `security@qh8z.com` mailbox/alias.
- real `ADMIN_EMAIL` and one-time `ADMIN_BOOTSTRAP_SECRET`.
- real legal operator name/jurisdiction for Terms and Privacy rendering.
- `TERMS_VERSION` matching the published Terms revision.

Run the non-secret-printing preflight before starting production:

```bash
bash scripts/preflight.sh .env
```

Public mode also performs application-level startup/readiness checks. Missing Web Risk, Turnstile, email verification, secure-cookie/HTTPS settings, SMTP, legal operator metadata, or administrator MFA prevents a healthy public launch.

## 3. Email deliverability

Configure SPF and DKIM for the sending domain and publish an appropriate DMARC policy. Verify delivery to major providers, spam-folder behavior, verification links, password resets, and bounce handling before opening signup.

Verification and reset tokens are one-time, stored only as hashes, and placed in URL fragments so raw tokens are not sent in normal HTTP request paths/referrers.

## 4. Start production

```bash
docker compose --env-file .env --profile production pull
docker compose --env-file .env --profile production up -d --build
```

Check Caddy, app, Shlink, and PostgreSQL logs after startup. `/healthz` is liveness. `/readyz` verifies the database, redirect engine, SMTP, public configuration, and administrator MFA.

## 5. Bootstrap and lock down the administrator

The configured admin email is reserved and cannot be registered over public HTTP. Bootstrap it from the host only:

```bash
bash scripts/bootstrap-admin.sh .env
```

Then sign in through the normal QH8Z login, enable authenticator-app MFA from **Account**, and save the one-time recovery codes offline. In public mode `/readyz` deliberately remains unhealthy until at least one administrator exists and every administrator has MFA enabled.

After bootstrap, rotate `ADMIN_BOOTSTRAP_SECRET` to a new random value and restart. Never expose the bootstrap secret, Shlink API key, MFA encryption key, or database credentials to a browser/client.

## 6. Functional smoke test

Before announcing the site, test from outside the host/network:

- register a normal account and verify its email;
- verify unverified accounts cannot create links;
- create generated and custom-alias links;
- confirm reserved aliases and private/local-network destinations are rejected;
- verify Google Web Risk fail-closed behavior and Turnstile enforcement;
- redirect through `https://qh8z.com/<slug>` and confirm visit counts;
- edit a destination and verify the same short URL changes target;
- generate/scan a QR code;
- submit an abuse report and action it from the admin queue;
- test user suspension and confirm active links stop redirecting;
- test forgot/reset-password and session invalidation;
- test administrator MFA login and a recovery code;
- test account export/deletion;
- validate `/security`, `/.well-known/security.txt`, Terms, Privacy, and Report Abuse pages.

## 7. Billing (when paid plans are enabled)

Stripe configuration is all-or-none. Set `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, and `STRIPE_PRO_PRICE_ID`. Configure a recurring Pro price and webhook endpoint at `https://qh8z.com/api/billing/webhook` for checkout/subscription events. Webhook event IDs are stored for idempotency and failed processing remains retryable.

Test checkout, plan activation, portal access, cancellation, failed/past-due behavior, duplicate webhook delivery, and account deletion with an active subscription.

## 8. Backups and monitoring

Create encrypted, off-host backups on a schedule. The repository includes:

```bash
bash scripts/backup.sh /secure/local/staging
CONFIRM_RESTORE=YES bash scripts/restore.sh /path/to/qh8z-postgres-*.sql.gz
```

Keep multiple restore points and perform a restore drill before public announcement and periodically afterward. A backup that has never been restored is not considered verified.

Monitor at minimum HTTPS uptime, `/readyz`, host disk/RAM, PostgreSQL storage, 5xx rate, SMTP failures, Web Risk failures, abuse queue age, certificate renewal, recurring reputation-worker failures, and backup success. Alert somebody who will actually respond.

## 9. Trust, abuse, and legal operations

Before broad marketing, ensure `support@qh8z.com`, `abuse@qh8z.com`, and `security@qh8z.com` are monitored. Establish an abuse-response SLA, escalation process, law-enforcement/data-request process, retention policy, and incident-response owner.

The repository includes substantial operational Privacy/Terms text, but the operator should still obtain jurisdiction-specific legal review for the actual business entity, tax/payment setup, consumer law, privacy obligations, age requirements, and intended markets before relying on those documents as legal advice.

## 10. Go/no-go

Public signup is a **go** only when all of these are true:

- CI, dependency audit, and Docker integration are green on the exact deploy commit.
- deterministic dependency installation succeeds from the committed lockfile.
- `scripts/preflight.sh` passes on the production host.
- administrator MFA is enrolled and `/readyz` is healthy.
- `scripts/postdeploy.sh` passes over the public internet.
- Web Risk, recurring reputation scans, Turnstile, SMTP verification/reset mail, support/abuse/security inboxes, backups, and monitoring are live.
- DNS/TLS are stable.
- Terms/Privacy reflect the actual operator and have received appropriate review.
- an administrator can respond to abuse and outages.

If any item is false, keep `PUBLIC_LAUNCH_MODE` off or keep signup closed.
