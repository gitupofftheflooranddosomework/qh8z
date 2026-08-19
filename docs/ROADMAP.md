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

- [x] Custom domains and DNS verification
- [x] Link disabling
- [ ] Link expiration
- [ ] Tags
- [x] QR codes
- [ ] Bulk import/export
- [ ] UTM builder
- [ ] Destination edit history
- [ ] Webhooks

## Milestone 5 — analytics

- [ ] Bot filtering
- [x] Referrer capture
- [ ] Browser / OS / device classification
- [ ] Country / region enrichment with privacy review
- [x] Daily time-series analytics
- [x] Plan-based retention windows
- [x] Dashboard
- [ ] CSV export

## Milestone 6 — monetization

- [x] Plan and entitlement model
- [x] Billing provider abstraction
- [x] Usage metering
- [x] Free and paid tiers
- [x] Custom-domain and analytics-retention entitlements
- [ ] Trials
- [x] Past-due grace behavior
- [x] Billing portal

## Milestone 7 — abuse resistance and launch readiness

- [x] Per-IP, account, and API-key rate limits
- [x] URL reputation / malware checks
- [x] Blocklists and allowlists
- [x] Abuse reporting workflow
- [x] Suspension and review tooling
- [x] Terms, privacy, and acceptable-use policies
- [x] Observability and incident runbooks
- [x] Load testing

Repository-side launch readiness is implemented. The remaining Issue #3 work is the live-environment verification in [`LAUNCH_RUNBOOK.md`](LAUNCH_RUNBOOK.md): production DNS/TLS, provider and billing checks, monitored role mailboxes, live abuse/log validation, and the final public qh8z.com smoke test.

## Post-launch candidates

Unchecked items above are product enhancements rather than Issue #3 launch blockers unless a later launch decision explicitly promotes them into the gate.
