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
- Active destinations are rechecked periodically and are automatically disabled if they later become unsafe or violate destination-network policy.
- QH8Z rejects loopback/private/reserved literal-network destinations and self-shortening targets.
- Shlink's API key and PostgreSQL are server-side only. Stored redirect analytics are configured to anonymize visitor IP addresses.
- Public app/redirect traffic terminates at Caddy; product and redirect services are not directly exposed to the public network.
- Reserved product routes cannot be claimed as custom short-link aliases.
- Administrator bootstrap is host-only and requires a separate one-time secret. Public administrators must enroll TOTP MFA before readiness becomes healthy.
- MFA secrets are encrypted at rest with a separate key; recovery codes are stored only as hashes.
- User suspension revokes sessions and disables active links.
- Abuse/moderation and significant account actions are recorded in an audit trail.
- Stripe webhook events are processed idempotently when billing is enabled.
- `/healthz` is a liveness endpoint; `/readyz` verifies critical dependencies/configuration and administrator MFA in public mode.

## Operations

Production readiness additionally depends on maintained dependencies, TLS, monitored security/abuse mailboxes, off-host tested backups, observability/alerting, secret rotation, and an incident-response process. See `docs/LAUNCH.md`.
