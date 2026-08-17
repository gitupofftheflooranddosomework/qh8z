# QH8Z Security

Please do not publish exploit details or active malicious QH8Z links in a public issue.

For the private beta, security and abuse reports should be sent to **support@qh8z.com** or through `/report` for harmful short links.

## Security posture

- Anonymous link creation is not supported by the application API.
- Link creation is tied to authenticated QH8Z accounts.
- Session tokens are random, stored server-side only as SHA-256 hashes, and delivered in HttpOnly SameSite cookies.
- Passwords are hashed with bcrypt.
- Shlink's API key is server-side only and is never exposed to the browser.
- Mutating browser requests are protected by same-origin checks and rate limits.
- Abuse reports and admin actions are recorded in the QH8Z database/audit trail.
- Production traffic terminates TLS at Caddy; short-code redirects are routed directly to Shlink.

Before broad public launch, add automated URL reputation scanning and a dedicated monitored security mailbox.
