# QH8Z Security

Please do not publish exploit details, credentials, private user data, or active malicious QH8Z links in a public issue.

## Reporting a vulnerability

Use `security@qh8z.com` for security vulnerabilities. Harmful short links should be reported through `https://qh8z.com/report` or `abuse@qh8z.com`.

QH8Z publishes a security contact document at `https://qh8z.com/.well-known/security.txt`.

## Security posture

- Public link creation requires an authenticated, non-suspended account with current Terms acceptance and, in public mode, a verified email.
- Passwords are bcrypt-hashed; session and auth/recovery tokens are random and only hashes are persisted.
- Production uses Secure HttpOnly `__Host-` session cookies and same-origin enforcement for cookie-authenticated mutations.
- Cloudflare Turnstile is server-validated for public signup/login/recovery/abuse forms in public mode.
- Google Web Risk checks link destinations at creation and edit time; public mode fails closed if reputation checking is unavailable.
- Active destinations are rechecked periodically and are automatically disabled if they later become unsafe or violate destination policy. Public mode rejects configuration that would silently disable recurring scans.
- QH8Z rejects self-shortening targets, loopback/private/reserved networks, IPv4-mapped IPv6 literals, single-label/internal hostnames, and reserved local/test hostname suffixes.
- Shlink's API key is server-side only. Stored redirect analytics are configured to anonymize visitor IP addresses, and the shipped Caddy edge does not enable per-request access logging.
- Public app/redirect traffic terminates at Caddy; product and redirect services are not directly exposed to the public network. Shlink's exact `/rest` and `/rest/*` management namespace returns 404 at the public edge.
- Reserved product/upstream routes cannot be claimed as custom short-link aliases.
- PostgreSQL has no public port mapping. The maintenance `postgres` role is not provided to QH8Z or Shlink; the two services use separate `qh8z_app` and `shlink_app` credentials and cannot connect to each other's database.
- Database connection, statement, and query waits are bounded so a dead database path fails instead of hanging indefinitely.
- Administrator bootstrap is host-only and requires a separate one-time secret. Public administrators must enroll TOTP MFA before readiness becomes healthy.
- MFA secrets are encrypted at rest with a separate key; recovery codes are stored only as hashes and consumed atomically. MFA-protected sensitive account actions require second-factor proof.
- User suspension revokes sessions and disables active links.
- Abuse/moderation and significant account actions are recorded in an audit trail. Audit-storage failures are emitted as structured operational errors rather than corrupting an already-completed business mutation.
- Stripe webhook event idempotency, subscription/account updates, and billing audit writes are committed in one database transaction when billing is enabled, so failed processing remains retryable.
- Backup/restore tooling preserves separate database ownership/connect restrictions and fails if a completed backup cannot return previously running services.
- `/healthz` is a liveness endpoint; `/readyz` verifies critical dependencies/configuration and administrator MFA in public mode.
- Deployment, rollback, and public post-deploy probes have bounded network timeouts rather than waiting indefinitely on half-open dependencies.

## Operations

Production readiness additionally depends on maintained dependencies, TLS, monitored security/abuse mailboxes, off-host tested encrypted backups, observability/alerting, secret rotation, and an incident-response process. See `docs/LAUNCH.md`.
