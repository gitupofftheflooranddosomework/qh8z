# qh8z roadmap

## Milestone 1 — working core

- [x] Link creation API
- [x] Generated slugs
- [x] Custom slugs
- [x] Redirect handling
- [x] Basic visit counts
- [x] Minimal web UI
- [x] Docker build
- [x] CI

## Milestone 2 — durable production data

- [x] PostgreSQL schema and migrations
- [x] Durable links and analytics
- [ ] Redis-compatible redirect cache
- [x] Backups and restore procedure
- [ ] Async analytics event pipeline

## Milestone 3 — accounts and ownership

- [x] Accounts and login
- [x] Verified email
- [x] Workspaces / organizations
- [x] Memberships and roles
- [x] Scoped API keys
- [x] Audit log

## Milestone 4 — serious link management

- [ ] Custom domains and DNS verification
- [ ] Link expiration and disabling
- [ ] Tags
- [ ] QR codes
- [ ] Bulk import/export
- [ ] UTM builder
- [ ] Destination edit history
- [ ] Webhooks

## Milestone 5 — analytics

- [ ] Bot filtering
- [x] Referrer capture
- [ ] Browser / OS / device classification
- [ ] Country / region enrichment with privacy review
- [ ] Time-series rollups
- [ ] Retention tiers
- [ ] Dashboard and CSV export

## Milestone 6 — monetization

- [ ] Plan and entitlement model
- [ ] Billing provider abstraction
- [ ] Usage metering
- [ ] Free and paid tiers
- [ ] Custom-domain and analytics-retention entitlements
- [ ] Trials and grace periods
- [ ] Billing portal

## Milestone 7 — abuse resistance and launch readiness

- [x] Per-IP, account, and API-key rate limits
- [x] URL reputation / malware checks
- [x] Blocklists and allowlists
- [x] Abuse reporting workflow
- [x] Suspension and review tooling
- [ ] Terms, privacy, and acceptable-use policies
- [ ] Observability and incident runbooks
- [ ] Load testing
