# Public-service safety controls

qh8z treats abuse prevention as a launch requirement, not an optional add-on. Production startup fails unless the public-service safety dependencies are configured.

## Rate limiting

Rate-limit counters are stored in PostgreSQL so limits are shared across application instances. Fixed windows currently enforce:

- all `/api/` requests: 300 requests/minute per client IP
- registration: 10 requests/hour per client IP
- login: 60 requests/15 minutes per client IP
- authenticated user activity: 300 requests/minute per user
- API keys: 600 requests/minute per key
- abuse reports: 20 requests/hour per client IP
- admin API: 60 requests/minute per client IP

Raw client IPs are not stored in the rate-limit table. qh8z stores a SHA-256 bucket derived from `QH8Z_RATE_LIMIT_SALT` and the address. Expired rate windows are cleaned opportunistically.

`X-Forwarded-For` is trusted only when the immediate peer belongs to a CIDR configured in `QH8Z_TRUSTED_PROXIES`. Configure only reverse proxies/load balancers you operate.

## Destination validation

Before a destination can be shortened, qh8z requires HTTP or HTTPS and rejects:

- embedded URL credentials
- localhost and special-use local/reserved hostnames
- private, loopback, link-local, multicast, unspecified, CGNAT, documentation, and benchmark IP ranges
- single-label hosts
- legacy numeric IP-like hosts
- invalid ports

Managed URL rules are then evaluated, followed by the reputation provider. A managed allow rule can skip reputation lookup for a specific public host/domain, but it cannot override the static local/private-address checks.

qh8z does not fetch a destination on behalf of the user, which avoids a server-side request-forgery path in shortening and redirect handling. DNS ownership/resolution can change after link creation, so abuse monitoring and link suspension remain necessary even after creation-time checks.

## Google Web Risk

Production requires `QH8Z_REPUTATION_MODE=webrisk` and `WEBRISK_API_KEY`.

The Lookup integration checks these threat types:

- malware
- social engineering
- unwanted software
- optionally extended social-engineering coverage with `WEBRISK_EXTENDED_COVERAGE=true`

Safe lookups are cached in-process for five minutes. Threat results honor the provider expiry when present. The cache is bounded. Reputation lookup errors fail closed for new link creation rather than silently bypassing safety.

## Managed allow/block rules

The admin API can create exact-host or domain/subdomain rules:

- `GET /api/v1/admin/url-rules`
- `POST /api/v1/admin/url-rules`
- `DELETE /api/v1/admin/url-rules/{id}`

Admin requests use `Authorization: Bearer $QH8Z_ADMIN_TOKEN`. Keep this token in the production secrets manager and rotate it as an operational credential.

## Abuse reporting and review

Anyone can submit:

```text
POST /api/v1/abuse-reports
```

with a short-link slug and category (`malware`, `phishing`, `scam`, `spam`, or `other`). Reports may include details and a reporter email. Unknown slugs receive the same accepted response shape so the endpoint is not useful as a link-existence oracle.

Admins can review reports at:

- `GET /api/v1/admin/abuse-reports`
- `PATCH /api/v1/admin/abuse-reports/{id}`

Links can be immediately removed from the public redirect path through:

- `POST /api/v1/admin/links/{slug}/suspend`
- `POST /api/v1/admin/links/{slug}/unsuspend`

Suspension and admin safety changes are included in audit history where applicable.

## Production environment

The safety subsystem requires these production settings:

- `QH8Z_ADMIN_TOKEN` — at least 32 bytes
- `QH8Z_RATE_LIMIT_SALT` — at least 32 bytes and secret
- `QH8Z_REPUTATION_MODE=webrisk`
- `WEBRISK_API_KEY`
- optional `WEBRISK_EXTENDED_COVERAGE=true`
- optional `QH8Z_TRUSTED_PROXIES=cidr,cidr,...`

Gate 5 still needs to put these secrets into the chosen production secrets-management system and add monitoring/alerting around abuse and reputation-provider failures.
