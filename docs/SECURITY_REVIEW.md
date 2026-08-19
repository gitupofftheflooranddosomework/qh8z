# qh8z launch security review

**Review updated: August 19, 2026**

This is the launch security review for Issue #3. It is an engineering review of the qh8z implementation and deployment controls; it is not a claim of third-party certification or a substitute for an independent penetration test.

## Release rule

A finding marked **Blocker** must be closed before the Issue #3 security-review checkbox can be marked complete. A production-only blocker may only be waived by an explicit documented risk-acceptance decision; repository completion alone does not waive a live-environment check.

## Gate 5 acceptance evidence

**Resolved**

Production-operations PR #6 was accepted on exact head `e6e11b46fa8c81607c706cb52a2ce2d77d978c5c` by GitHub Actions CI run #377 and squash-merged to `main`.

That exact-head run passed module locking, formatting, `go vet`, race-enabled application/PostgreSQL tests, production builds, development and production Compose validation, Caddy validation, Prometheus/Alertmanager validation, operator-script validation, application and backup image builds, database-backed readiness, redirect load testing, encrypted restic backup/restore, and deliberate PostgreSQL outage/recovery.

The rehearsal produced 3,326 redirects in three seconds with zero failures, approximately 1,108.7 requests/second, and p95 latency of 16.32 ms. The seeded link was restored into a fresh database, readiness failed while PostgreSQL was intentionally offline, readiness recovered after PostgreSQL returned, and qh8z recovered after an application restart.

## Identity and authorization

**Resolved in launch candidate**

- Browser sessions and API keys are separate credential types.
- Links are workspace-owned.
- API keys are scoped and only hashed secrets are stored.
- Session and verification secrets are stored as hashes.
- Verified email is required before link creation.
- Owner/admin checks protect billing and workspace administration.
- Sensitive workspace, link, API-key, billing, and admin operations are audited.
- `security_matrix_test.go` exercises cross-workspace links, domains, analytics, billing, members, API keys, and audit access; object-level cross-workspace reads/writes; API-key scope failures; API-key workspace mismatch; and logout/session invalidation.

## Destination and redirect security

**Resolved in launch candidate**

- Only HTTP/HTTPS destinations are accepted.
- URLs containing credentials are rejected.
- Localhost, private, link-local, reserved, single-label/internal, legacy numeric, and unsafe IPv4/IPv6 forms are rejected.
- Production reputation checks fail closed for new-link creation.
- Link edits repeat destination/reputation validation.
- Suspended/disabled links are removed from public redirects.
- `destination_matrix_test.go` exercises unsafe schemes, private/reserved networks, IPv4/IPv6 variants, numeric host encodings, credentials, invalid ports, reserved names, and allowed public controls through the real create-link route.

## Abuse resistance

**Pass in implementation**

- PostgreSQL-backed fixed-window rate limiting exists for IP, account, and API-key principals.
- Durable IP rate-limit keys are salted hashes rather than raw IP strings.
- Public abuse reporting, internal review, URL rules, suspension, and audit history are implemented.
- Gate 6 publishes a no-account `/report-abuse` form backed by the rate-limited abuse API.
- Production requires a strong admin token and rate-limit salt.

**Blocker — live launch environment**

- Submit a real abuse report and complete review → suspension → redirect blocked → resolution.
- Confirm Caddy trusted-proxy/source-IP behavior produces the actual client bucket rather than the proxy/container address.

## Browser, HTTP, TLS, and custom-domain security

**Pass in implementation**

- Session cookies are HTTP-only and production cookies require HTTPS.
- The application sets content-type, frame, referrer, permissions, and CSP headers.
- Production base URL must use HTTPS.
- Caddy is the only public-facing service in the reference deployment.
- Public Caddy rules block `/metrics`, `/readyz`, and `/internal/*`.
- On-demand certificate issuance uses the internal qh8z authorization endpoint and only verified, currently entitled Pro custom domains are accepted.
- Free → denied, active Pro → allowed, past-due grace → allowed, and canceled Pro → denied are covered by automated tests.

## Secrets and providers

**Resolved in repository**

- qh8z runs non-root in production.
- File-mounted secrets use the documented `root:65532` / `0640` model.
- Docker build context excludes local secret/config state.
- Production startup refuses missing PostgreSQL, SMTP, Web Risk, admin/rate-limit, or Stripe requirements.
- Stripe webhook signatures are verified against the raw body with timestamp tolerance.
- Stripe webhook event IDs are durably claimed for idempotency and released on failed processing.
- The full-Git-history Secret Scan workflow detects provider-shaped keys and opaque direct secret assignments. Historical `change-me-*` example placeholders are explicitly treated as non-secret fixtures rather than weakening the real-secret rules.

**Blocker — live launch environment**

- Verify production provider keys use the minimum practical permissions and can be independently rotated.
- Verify the production Stripe webhook targets the exact qh8z endpoint and rejects an invalid signature.

## Billing and entitlement security

**Resolved in launch candidate**

- Free/Pro limits are enforced server-side.
- Billing management requires owner/admin authorization.
- Custom-domain creation requires Pro.
- Canceled Pro workspaces fall back to Free entitlements.
- Per-request custom-domain entitlement enforcement prevents a cached certificate from preserving branded traffic after cancellation.
- `launch_flow_test.go` runs registration/email verification → shorten → redirect → analytics → QR → Stripe Checkout → signed webhook → Pro entitlement → signed cancellation webhook → Free fallback through real qh8z routes and the real Stripe adapter against an isolated provider test server.

**Blocker — live launch environment**

- Complete one real Stripe test/live-mode subscription flow against the deployed launch environment, including portal cancellation and branded-domain denial after the cancellation webhook.

## Data protection and recovery

**Pass**

- PostgreSQL is private to the internal production network.
- Production backups use `pg_dump` plus encrypted restic storage.
- Restore requires an explicit confirmation value.
- Backup retention, deployment backups, restore drills, rollback, readiness, and failure procedures are documented.
- Gate 5 CI created an encrypted backup, restored it into a fresh database, verified the seeded link, and proved readiness failure/recovery through a deliberate PostgreSQL outage.

## Logging and privacy

**Pass in design/implementation**

- Production application logs are structured JSON.
- Caddy access logs use bounded Docker log rotation.
- Prometheus metrics avoid destination URLs, user emails, API secrets, and IP-address labels.
- Visitor IPs are not required for product analytics and durable rate-limit IP keys are hashed.

**Blocker — live launch environment**

- Inspect representative qh8z, Caddy, PostgreSQL, Prometheus, and Alertmanager output and confirm passwords, session tokens, API-key secrets, Stripe secrets, Web Risk keys, and email-verification tokens are not logged.

## Dependency and build security

**Launch gate**

- Go module files are locked and CI rejects `go mod tidy` drift.
- qh8z builds with Go 1.25.13 and patched indirect dependencies.
- `govulncheck` scans reachable Go vulnerabilities.
- Trivy scans every production runtime image for fixable HIGH/CRITICAL vulnerabilities with `--ignore-unfixed`; findings fail the workflow rather than being allowlisted.
- qh8z uses a scratch runtime image and non-root numeric user.
- Caddy is rebuilt on the patched qh8z Go/Alpine baseline because the pinned upstream image did not satisfy the launch scan.
- PostgreSQL retains the official 17-alpine base but replaces/removes the vulnerable `gosu` helper with `su-exec` while preserving the official entrypoint behavior.
- The backup worker is a minimal Alpine client image rather than a PostgreSQL server image.
- Prometheus `v3.13.2` and Alertmanager `v0.33.1` are pinned upstream releases and are scanned as the exact images used by production Compose.

The dependency/container portion is resolved only when the Security Audit workflow is green on the final PR head that is merged.

## Security-reporting process

**Implemented**

- `SECURITY.md` defines private vulnerability reporting and safe-research expectations.
- `/security` publishes that source-of-truth policy.
- `/.well-known/security.txt` publishes the canonical security contact and policy URL.

**Blocker — live launch environment**

- `security@qh8z.com` must accept external mail and be monitored before public launch.

## Remaining blockers that require the real production environment

1. Real abuse report → suspension → resolution exercise plus source-IP verification.
2. Provider least-privilege/rotation review and real Stripe webhook/subscription/cancellation test.
3. Representative production log inspection for credential/token leakage.
4. External delivery and monitoring verification for `security@qh8z.com` and the other published role contacts.
5. Successful public DNS/TLS/application smoke test for `qh8z.com` after the production VPS address is available and DNS is pointed at it.

The Issue #3 **Security review** checkbox stays open until those live-environment blockers are completed or explicitly waived with documented risk acceptance.
