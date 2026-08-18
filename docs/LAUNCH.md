# QH8Z public launch runbook

QH8Z is designed to fail closed in `PUBLIC_LAUNCH_MODE=true` when critical public-safety services are missing. The application code can be public-launch-ready while DNS, credentials, mail reputation, and legal/business operations remain external deployment requirements.

## 1. Provision the host

Use a maintained Linux host with Docker Engine and Compose, persistent storage, automatic security updates, an allowlist firewall, and enough disk for PostgreSQL/visit growth. Expose only TCP 80/443 and UDP 443 publicly. PostgreSQL, QH8Z port 3000, and Shlink port 8080 must remain private/loopback.

Point `qh8z.com` and `www.qh8z.com` to the host. Caddy obtains and renews TLS certificates automatically.

## 2. Configure production secrets and dependencies

```bash
cp .env.production.example .env
openssl rand -hex 32   # POSTGRES_PASSWORD (maintenance/admin only)
openssl rand -hex 32   # QH8Z_DB_PASSWORD
openssl rand -hex 32   # SHLINK_DB_PASSWORD
openssl rand -hex 32   # SHLINK_API_KEY
openssl rand -hex 32   # ADMIN_BOOTSTRAP_SECRET (one-time setup)
openssl rand -hex 32   # MFA_ENCRYPTION_KEY (persistent; back this up securely)
```

Use a different value for every secret. `POSTGRES_PASSWORD` belongs only to the PostgreSQL maintenance account. QH8Z connects as `qh8z_app` with `QH8Z_DB_PASSWORD`; Shlink connects as `shlink_app` with `SHLINK_DB_PASSWORD`. The two application roles are not allowed to connect to each other's database.

Fill every required field in `.env`:

- Google Web Risk API key, with `WEB_RISK_REQUIRED=true`.
- Cloudflare Turnstile site/secret keys, with `TURNSTILE_REQUIRED=true`.
- SMTP host/credentials and a domain-authenticated `MAIL_FROM` address.
- monitored `SUPPORT_EMAIL`, `ABUSE_EMAIL`, and `security@qh8z.com` mailbox/alias.
- real `ADMIN_EMAIL`; a strong `ADMIN_BOOTSTRAP_SECRET` is needed until the first admin exists.
- real legal operator name/jurisdiction for Terms and Privacy rendering.
- `TERMS_VERSION` matching the published Terms revision.
- `SESSION_TTL_DAYS` between 1 and 90 and `ADMIN_SESSION_HOURS` between 1 and 24; templates default to 30 days and 12 hours respectively.
- positive recurring reputation-scan settings; public mode refuses settings that silently disable the worker.

Run the non-secret-printing preflight before starting production:

```bash
bash scripts/preflight.sh .env
```

Public mode also performs application-level startup/readiness checks. Missing Web Risk, Turnstile, email verification, secure-cookie/HTTPS settings, SMTP, legal operator metadata, valid runtime intervals, or administrator MFA prevents a healthy public launch. Preflight additionally enforces the documented 64-hex-character format for database and Shlink API secrets so connection-string parsing cannot be broken by URL-reserved password characters.

## 3. Email deliverability

Configure SPF and DKIM for the sending domain and publish an appropriate DMARC policy. Verify delivery to major providers, spam-folder behavior, verification links, password resets, and bounce handling before opening signup.

Verification and reset tokens are one-time, stored only as hashes, and placed in URL fragments so raw tokens are not sent in normal HTTP request paths/referrers.

## 4. Start production

```bash
docker compose --env-file .env --profile production pull
docker compose --env-file .env --profile production up -d --build --remove-orphans
```

Check Caddy, app, Shlink, and PostgreSQL logs after startup. `/healthz` is liveness. `/readyz` verifies the database, redirect engine, SMTP, public configuration, and administrator MFA. Shlink's exact `/rest` path and `/rest/*` management surface are blocked at the public Caddy edge.

## 5. Bootstrap and lock down the administrator

The configured admin email is reserved and cannot be registered over public HTTP. Bootstrap it from the host only:

```bash
bash scripts/bootstrap-admin.sh .env
```

Then sign in through the normal QH8Z login, enable authenticator-app MFA from **Account**, and save the one-time recovery codes offline. In public mode `/readyz` deliberately remains unhealthy until at least one administrator exists and every administrator has MFA enabled.

After bootstrap, remove `ADMIN_BOOTSTRAP_SECRET` from `.env` and restart. The app only requires it when no administrator exists. Keep `MFA_ENCRYPTION_KEY` stable and backed up securely; changing or losing it makes existing encrypted authenticator secrets unusable.

Never expose the bootstrap secret, Shlink API key, MFA encryption key, PostgreSQL administrator credential, or service database credentials to a browser/client.

## 6. Functional smoke test

Before announcing the site, test from outside the host/network:

- register a normal account and verify its email;
- verify unverified accounts cannot create links;
- create generated and custom-alias links;
- confirm reserved aliases, private/reserved IPs, IPv4-mapped IPv6, single-label hosts, and local/internal hostname suffixes are rejected;
- verify Google Web Risk fail-closed behavior and Turnstile enforcement;
- redirect through `https://qh8z.com/<slug>` and confirm visit counts;
- edit a destination and verify the same short URL changes target;
- verify `https://qh8z.com/rest` and `https://qh8z.com/rest/health` return 404 rather than exposing Shlink management APIs;
- generate/scan a QR code;
- submit an abuse report and action it from the admin queue;
- test user suspension and confirm active links stop redirecting;
- test forgot/reset-password and session invalidation;
- test administrator MFA login and a recovery code;
- verify MFA-protected password changes and account deletion require second-factor proof;
- test account export/deletion;
- validate `/security`, `/.well-known/security.txt`, Terms, Privacy, and Report Abuse pages.

## 7. Billing (when paid plans are enabled)

Stripe configuration is all-or-none. Set `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, and `STRIPE_PRO_PRICE_ID`. Configure a recurring Pro price and webhook endpoint at `https://qh8z.com/api/billing/webhook` for checkout/subscription events. Webhook idempotency insertion, account/subscription state changes, and billing audit writes are committed in one PostgreSQL transaction so a process failure does not permanently mark a partially processed event as complete.

Test checkout, plan activation, portal access, cancellation, failed/past-due behavior, duplicate webhook delivery, and account deletion with an active subscription.

## 8. Backups and monitoring

Create encrypted, off-host backups on a schedule. The repository's backup command briefly quiesces QH8Z, Shlink, and the public edge; produces separate PostgreSQL custom-format dumps for both databases using the maintenance role; packages them with a manifest; writes a SHA-256 checksum; and restores exactly the previously running service set without recreating healthy dependencies:

```bash
bash scripts/backup.sh /secure/local/staging .env
CONFIRM_RESTORE=YES bash scripts/restore.sh /secure/local/staging/qh8z-backup-YYYYMMDDTHHMMSSZ.tar.gz .env
```

`backup.sh` returns a failure if it cannot restore the previously running services after taking the snapshot. `restore.sh` verifies the checksum when present, rejects unsafe/malformed archives, stops application/redirect writers, recreates both databases under their separate application owners, reapplies cross-database CONNECT restrictions, restores with `pg_restore --exit-on-error`, and only then restarts services that were previously running. If a restore fails, application services intentionally remain stopped rather than serving a partial restore.

Encrypt backup archives before sending them off-host and keep multiple restore points. CI performs a destructive backup/restore drill that creates sentinel state in both databases, verifies the two service roles cannot cross-connect, deletes the sentinels, restores the archive, re-verifies role isolation, and confirms both the QH8Z record, Shlink redirect, and HTTPS edge return. Repeat a real production restore drill periodically; a CI drill does not replace testing your actual backup destination and credentials.

Monitor at minimum HTTPS uptime, `/readyz`, host disk/RAM, PostgreSQL storage, 5xx rate, SMTP failures, Web Risk failures, abuse queue age, certificate renewal, recurring reputation-worker failures, audit-write failures, and backup success/service-restoration failure. Alert somebody who will actually respond.

## 9. Trust, abuse, and legal operations

Before broad marketing, ensure `support@qh8z.com`, `abuse@qh8z.com`, and `security@qh8z.com` are monitored. Establish an abuse-response SLA, escalation process, law-enforcement/data-request process, retention policy, and incident-response owner.

The repository includes substantial operational Privacy/Terms text, but the operator should still obtain jurisdiction-specific legal review for the actual business entity, tax/payment setup, consumer law, privacy obligations, age requirements, and intended markets before relying on those documents as legal advice.

## 10. Go/no-go

Public signup is a **go** only when all of these are true:

- CI, production dependency audit, full HTTPS Docker integration, database-isolation assertions, and destructive backup/restore integration are green on the exact deploy commit.
- deterministic dependency installation succeeds from the committed lockfile.
- `scripts/preflight.sh` passes on the production host.
- administrator MFA is enrolled and `/readyz` is healthy.
- `scripts/postdeploy.sh` passes over the public internet without timing out.
- Web Risk, recurring reputation scans, Turnstile, SMTP verification/reset mail, support/abuse/security inboxes, backups, and monitoring are live.
- DNS/TLS are stable.
- Terms/Privacy reflect the actual operator and have received appropriate review.
- an administrator can respond to abuse and outages.

If any item is false, keep `PUBLIC_LAUNCH_MODE` off or keep signup closed.
