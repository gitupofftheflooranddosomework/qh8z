# QH8Z architecture

## Boundary, not a Frankenstein merge

QH8Z combines Shlink and Kutt at capability boundaries rather than concatenating both repositories. Shlink owns redirect correctness, short-code mechanics and raw visits. Kutt informs product patterns. QH8Z owns identity, verification/recovery, policy, plans, billing, moderation and presentation.

## Request paths

- Product/API: `Browser -> Caddy -> QH8Z -> QH8Z PostgreSQL -> Shlink REST API when needed`.
- Redirect: `Visitor -> Caddy -> Shlink -> destination`.

The hot redirect path is independent of the Node product app.

## Databases

One PostgreSQL service hosts two databases: `qh8z` and `shlink`, but the isolation is credential-enforced rather than just naming. The PostgreSQL `postgres` account is reserved for host maintenance/backup/restore. QH8Z connects only as `qh8z_app`; Shlink connects only as `shlink_app`. PUBLIC database CONNECT is revoked and each application role is granted CONNECT only to its own database. CI asserts both permitted connections and both cross-database denials before and after restore.

QH8Z does not query Shlink's private schema; application integration uses Shlink's documented REST API.

The QH8Z database stores users, hashed sessions, hashed one-time auth tokens, link ownership, plans, Stripe event idempotency, abuse reports, recurring reputation state, and audit events.

Database connections have finite connection/query/statement timeouts so dependency failure becomes a bounded error instead of an indefinitely waiting request.

## Identity and public-account eligibility

Passwords use bcrypt. Sessions use random 256-bit bearer tokens but PostgreSQL stores only SHA-256 hashes. Public production cookies are Secure, HttpOnly, SameSite=Lax and use the `__Host-` prefix.

Public link mutations require a non-suspended account, verified email, and acceptance of the current Terms version. Email verification and reset tokens are single-use, hashed at rest and delivered in URL fragments.

Turnstile is an independent public-form bot-abuse gate; the backend always verifies tokens in public mode.

Administrator bootstrap is host-only. Public administrators must enroll TOTP MFA before readiness becomes healthy. TOTP secrets are encrypted at rest with a separate AES-256-GCM key; recovery codes are stored only as hashes and consumed atomically. MFA-protected sensitive account changes require second-factor proof.

## Link ownership and destination policy

QH8Z stores each Shlink `short_code` with the owning user. That mapping is the authorization boundary.

Before create/edit, QH8Z rejects unsupported URLs, embedded credentials, self-shortening URLs, literal local/private/reserved networks, IPv4-mapped IPv6 literals, single-label/internal hostnames, and reserved local/test suffixes, then applies Google Web Risk. Public mode fails closed when that check cannot be completed.

Active destinations are rechecked in the background. A destination that later becomes unsafe or violates network policy is removed from the redirect engine and the QH8Z link record is disabled/audited. Public mode also refuses worker settings that would silently disable recurring rechecks.

Generated/custom aliases are prevented from colliding with QH8Z product or upstream-management routes.

## Routing, privacy, and observability

Caddy sends known product routes to QH8Z and everything else to Shlink, except Shlink's exact `/rest` and `/rest/*` management namespace, which is explicitly blocked at the public edge. App/Shlink direct ports are loopback-only in the Compose deployment. PostgreSQL has no public port mapping.

Shlink is configured to anonymize stored visitor IP addresses. The shipped Caddy configuration does not enable per-request access logging. QH8Z emits request IDs and structured application logs, exposes `/healthz` for liveness and `/readyz` for dependency/configuration readiness, and publishes security contact metadata under `/.well-known/security.txt`.

Operational audit telemetry logs failures without turning an already-completed business mutation into a contradictory 500. Stripe billing audit writes are different: they remain inside the same PostgreSQL transaction as webhook idempotency and subscription/account state so failed Stripe processing rolls back atomically.

## Billing and moderation

Stripe lives entirely in the QH8Z layer. Webhook event IDs, subscription/account changes, and billing audit records are committed in one transaction; failed processing remains retryable. Public abuse reports enter the QH8Z moderation queue. Admins can disable links or suspend users; suspending a user revokes sessions and removes active redirects. Significant actions produce audit events when audit storage is healthy and a structured error when it is not.

## Recovery and deployment

Backup quiesces all state writers and the public edge, dumps both databases with the maintenance account, then restores exactly the services that were previously running without recreating healthy dependencies. Restore recreates each database with the correct application owner and reapplies CONNECT restrictions before reopening traffic. A backup run fails if the previously running service set cannot be restored afterward.

Production deploy/rollback readiness and public post-deploy probes have bounded connection/response timeouts. A failed deployment retains the pre-deploy database snapshot and automatically returns application code to the previous commit.

## Upstream strategy

Pin Shlink releases and upgrade intentionally. Do not vendor all of Kutt. If upstream code is selectively adapted, keep provenance and MIT notices. The goal is an acquirable QH8Z product whose business layer is not coupled to an upstream UI or database schema.
