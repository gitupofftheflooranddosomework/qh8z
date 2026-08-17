# QH8Z Security

Please do not publish exploit details or active malicious QH8Z links in a public issue.

For the private beta, security and abuse reports should be sent to **support@qh8z.com** or through `/report` for harmful short links.

## Security posture

- Anonymous link creation is not supported by the application API.
- Link creation is tied to authenticated QH8Z accounts.
- Session tokens are random, stored server-side only as SHA-256 hashes, and delivered in HttpOnly SameSite cookies; production cookies are Secure.
- Passwords are hashed with bcrypt.
- Shlink's API key is server-side only and is never exposed to the browser.
- New and edited destinations pass through Google Web Risk when configured; production is intended to run with fail-closed reputation checking.
- Mutating browser requests are protected by same-origin checks and rate limits.
- The configured administrator email is reserved and requires a separate bootstrap secret for first registration.
- Abuse reports and admin actions are recorded in the QH8Z database/audit trail.
- Production traffic terminates TLS at Caddy; short-code redirects are routed directly to Shlink.
- The app and Shlink development ports bind to loopback rather than all interfaces.

## Before broad public launch

Operate a monitored security/abuse mailbox, retain and test off-host backups, add uptime/error monitoring and alerting, define log retention, and complete jurisdiction-specific legal/privacy review. Rotate the admin bootstrap secret after the administrator account has been created.
