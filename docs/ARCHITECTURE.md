# QH8Z architecture

## Decision

QH8Z is not a literal concatenation of Kutt and Shlink repositories. That would create duplicate link models, authentication concepts, analytics paths, and a painful upgrade story.

Instead, QH8Z combines them at the capability boundary:

- Shlink owns redirect correctness, short-code mechanics, and raw visit tracking.
- Kutt informs the product experience and feature set we want from a multi-user shortener.
- QH8Z owns customer identity, product policy, plans, monetization, moderation, and presentation.

This produces one coherent product and preserves a future path to replace Shlink without replacing the business.

## Request paths

Product/API: `Browser -> Caddy -> QH8Z Node app -> QH8Z PostgreSQL -> Shlink REST API when needed`.

Redirect: `Visitor -> Caddy -> Shlink -> destination`, with Shlink tracking the visit. The redirect hot path never needs the QH8Z application server.

## Databases

One PostgreSQL server hosts two databases: `qh8z` for users/sessions/ownership/plans/abuse/audit, and `shlink` for Shlink-owned schema and visit records. QH8Z never reaches into Shlink's private database schema; integration stays on the documented REST API.

## Identity and sessions

QH8Z uses its own accounts instead of exposing Shlink API keys. Passwords use bcrypt cost 12. Session tokens have 256 random bits; only a SHA-256 hash is stored in PostgreSQL. Cookies are HttpOnly/SameSite=Lax and Secure in production.

## Link ownership

Every link created by QH8Z is recorded with its Shlink `short_code`. That QH8Z mapping is the authorization boundary. Shlink remains the source of truth for redirect execution and visits.

## Routing

Caddy sends known product paths to QH8Z and every other path to Shlink. QH8Z therefore blocks custom aliases that would collide with product routes.

## Billing and abuse

Stripe is isolated in the QH8Z layer. Public abuse reports flow to the QH8Z moderation queue, where admins can review and disable links; all important moderation actions create audit events.

## Upstream strategy

Pin Shlink releases and upgrade deliberately. Do not vendor the entire Kutt app. If Kutt contains a useful implementation, port the smallest useful portion, preserve provenance and its MIT notice, and adapt it to the QH8Z model.
