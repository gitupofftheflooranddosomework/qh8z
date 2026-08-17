# QH8Z Architecture

## Goal

Build QH8Z as a commercially useful link-management product without coupling the business to a single open-source implementation.

## Decision: compose, do not Frankenstein

QH8Z will not run Kutt and Shlink as competing shortening engines.

- **Shlink** is the initial redirect/link engine.
- **QH8Z** owns the customer-facing product and business layer.
- **Kutt** is a selectively reused/reference source for useful product patterns and MIT-licensed implementation ideas.

This keeps one source of truth for redirects while allowing us to benefit from both projects.

## Boundaries

### Redirect engine — Shlink

Responsibilities:

- short-code resolution
- HTTP redirects
- core short-link persistence
- visit capture
- redirect rules supported by Shlink
- low-level link-management API

QH8Z must access Shlink through a narrow adapter/gateway instead of letting product code scatter raw Shlink calls everywhere.

### QH8Z product layer

Responsibilities planned for QH8Z-owned code:

- accounts and sessions
- organizations/workspaces
- roles and permissions
- subscription plans and billing
- usage quotas
- API tokens and customer-facing API
- dashboard/UI
- branded/custom domains
- QR workflows
- richer analytics presentation
- bulk operations
- abuse reports
- destination screening and domain reputation controls
- rate limiting and account reputation
- audit trail/admin tooling
- webhooks/integrations

### Kutt donor/reference role

Kutt contains useful existing implementations for several product-level concerns. We may selectively adapt MIT-licensed code where it materially accelerates QH8Z.

Any substantial copied/adapted code should:

1. be evaluated against the current QH8Z architecture rather than copied blindly;
2. have provenance retained in Git history and, where helpful, source comments;
3. preserve required MIT copyright/license notices;
4. avoid importing Kutt-specific branding or assumptions unnecessarily.

## Domain reputation

`qh8z.com` is an asset as well as infrastructure. At launch:

- no anonymous public link creation;
- creation goes through authenticated QH8Z APIs;
- restrict destinations to HTTP/HTTPS;
- add rate limits before opening registration;
- add destination/reputation checks before broad public access;
- maintain an abuse-reporting and rapid-disable path.

## Data ownership

Shlink owns engine-specific redirect/visit data initially. QH8Z will maintain separate product/customer data.

Do not encode billing, subscription, team, or customer identity concepts directly into Shlink tables. Link ownership should be mapped through QH8Z identifiers so the engine can be migrated later.

## Migration strategy

The desired long-term seam is:

```text
QH8Z product code -> LinkEngine interface -> Shlink adapter
                                      \-> future QH8Z-native/edge engine
```

Before replacing Shlink, provide an export/import path for:

- short codes
- destination URLs
- domains
- redirect rules
- creation timestamps
- link metadata
- aggregate analytics where useful

## Initial milestones

### M0 — Bootstrap

- [x] establish architecture
- [x] pin initial Shlink version
- [x] preserve third-party notices
- [x] create local PostgreSQL/Shlink stack
- [x] create minimal QH8Z gateway

### M1 — Private alpha

- [ ] establish stable Shlink API-key bootstrap
- [ ] QH8Z database/schema
- [ ] account authentication
- [ ] link ownership mapping
- [ ] create/list/edit/disable links
- [ ] first dashboard
- [ ] rate limiting
- [ ] abuse-disable workflow

### M2 — Monetizable beta

- [ ] organizations/workspaces
- [ ] Stripe billing
- [ ] free/pro/business plan enforcement
- [ ] custom domains
- [ ] QR codes
- [ ] analytics UI
- [ ] API keys and quotas
- [ ] terms/privacy/acceptable-use flows

### M3 — Public service

- [ ] destination reputation screening
- [ ] automated abuse detection
- [ ] reporting pipeline
- [ ] operational monitoring
- [ ] backups/disaster recovery
- [ ] production domain routing

## Upstream baselines

At bootstrap time:

- Shlink: `v5.1.5`
- Kutt: `v3.2.6`

These are reference points, not promises to track every upstream release automatically. Security and correctness fixes should be reviewed regularly.
