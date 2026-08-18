# QH8Z architecture

## Boundary, not a Frankenstein merge

QH8Z combines Shlink and Kutt at capability boundaries rather than concatenating both repositories. Shlink owns redirect correctness, short-code mechanics and raw visits. Kutt informs product patterns. QH8Z owns identity, verification/recovery, policy, plans, billing, moderation and presentation.

## Request paths

- Product/API: `Browser -> Caddy -> QH8Z -> QH8Z PostgreSQL -> Shlink REST API when needed`.
- Redirect: `Visitor -> Caddy -> Shlink -> destination`.

The hot redirect path is independent of the Node product app.

## Databases

One PostgreSQL service hosts two isolated databases: `qh8z` and `shlink`. QH8Z does not query Shlink's private schema; integration uses Shlink's documented REST API.

The QH8Z database stores users, hashed sessions, hashed one-time auth tokens, link ownership, plans, Stripe event idempotency, abuse reports, recurring reputation state, and audit events.

## Identity and public-account eligibility

Passwords use bcrypt. Sessions use random 256-bit bearer tokens but PostgreSQL stores only SHA-256 hashes. Public production cookies are Secure, HttpOnly, SameSite=Lax and use the `__Host-` prefix.

Public link mutations require a non-suspended account, verified email, and acceptance of the current Terms version. Email verification and reset tokens are single-use, hashed at rest and delivered in URL fragments.

Turnstile is an independent public-form bot-abuse gate; the backend always verifies tokens in public mode.

Administrator bootstrap is host-only. Public administrators must enroll TOTP MFA before readiness becomes healthy. TOTP secrets are encrypted at rest with a separate AES-256-GCM key; recovery codes are stored only as hashes.

## Link ownership and destination policy

QH8Z stores each Shlink `short_code` with the owning user. That mapping is the authorization boundary.

Before create/edit, QH8Z rejects unsupported URLs, embedded credentials, self-shortening URLs, and literal local/private/reserved network destinations, then applies Google Web Risk. Public mode fails closed when that check cannot be completed.

Active destinations are rechecked in the background. A destination that later becomes unsafe or violates network policy is removed from the redirect engine and the QH8Z link record is disabled/audited.

Generated/custom aliases are prevented from colliding with QH8Z product routes.

## Routing, privacy, and observability

Caddy sends known product routes to QH8Z and everything else to Shlink. App/Shlink direct ports are loopback-only in the Compose deployment. PostgreSQL has no public port mapping.

Shlink is configured to anonymize stored visitor IP addresses. QH8Z emits request IDs and structured request logs, exposes `/healthz` for liveness and `/readyz` for dependency/configuration readiness, and publishes security contact metadata under `/.well-known/security.txt`.

## Billing and moderation

Stripe lives entirely in the QH8Z layer. Webhook event IDs provide idempotency and failed processing is retryable. Public abuse reports enter the QH8Z moderation queue. Admins can disable links or suspend users; suspending a user revokes sessions and removes active redirects. Significant actions produce audit events.

## Upstream strategy

Pin Shlink releases and upgrade intentionally. Do not vendor all of Kutt. If upstream code is selectively adapted, keep provenance and MIT notices. The goal is an acquirable QH8Z product whose business layer is not coupled to an upstream UI or database schema.
