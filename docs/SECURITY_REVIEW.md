# qh8z launch security review

**Review updated: August 19, 2026**

This is the launch security review for Issue #3. It is an engineering review of the qh8z implementation and deployment controls; it is not a claim of third-party certification or a substitute for an independent penetration test.

## Release rule

A finding marked **Blocker** must be closed before the Issue #3 security-review checkbox can be marked complete. Medium/low post-launch improvements may remain only when their risk, owner, and mitigation are documented.

## Gate 5 acceptance evidence

**Resolved**

Production-operations PR #6 was accepted on exact head `e6e11b46fa8c81607c706cb52a2ce2d77d978c5c` by GitHub Actions CI run #377 and squash-merged to `main`.

That exact-head run passed module locking, formatting, `go vet`, race-enabled application/PostgreSQL tests, production builds, development and production Compose validation, Caddy 2.11.4 validation, Prometheus/Alertmanager validation, operator-script validation, application and backup image builds, database-backed readiness, redirect load testing, encrypted restic backup/restore, and deliberate PostgreSQL outage/recovery.

The rehearsal produced 3,326 redirects in three seconds with zero failures, approximately 1,108.7 requests/second, and p95 latency of 16.32 ms. The seeded link was restored into a fresh database, readiness failed while PostgreSQL was intentionally offline, readiness recovered after PostgreSQL returned, and qh8z recovered after an application restart.

These results close the former Gate 5 proxy, backup/restore, load, failure-recovery, and cached-certificate entitlement blockers in this review.

## Identity and authorization

**Pass in implementation**

- Browser sessions and API keys are separate credential types.
- Links are workspace-owned rather than globally editable by any authenticated account.
- API keys are scoped and store only a hash of the issued secret.
- Session/verification secrets are stored as hashes rather than plaintext tokens.
- Email verification is required before link creation.
- Workspace owner/admin checks protect billing and workspace-administration operations.
- Audit records exist for sensitive workspace, link, API-key, billing, and admin actions.

**Blocker — launch-candidate verification**

- Attempt cross-workspace reads/writes for links, domains, analytics, members, API keys, audit, and billing and confirm `403`/`404` as appropriate.
- Attempt every API-key scope boundary with a key missing the required scope.
- Confirm logout invalidates the active browser session.

## Destination and redirect security

**Pass in implementation**

- Only HTTP/HTTPS destinations are accepted.
- URLs containing credentials are rejected.
- Localhost, private, link-local, reserved, and numeric-IP-like destinations are rejected to reduce SSRF/open-redirect abuse against internal services.
- URL reputation checks are required in production and failures fail closed for new link creation.
- Managed block/allow rules exist; static private/local destination protections are not bypassed by an allow rule.
- Link edits repeat destination/reputation validation rather than trusting a previously-created link.
- Suspended/disabled links are removed from public redirects.

**Blocker — launch-candidate verification**

- Repeat the unsafe-destination test matrix against the production binary/proxy, including IPv4, IPv6, encoded/numeric host forms, credentials, invalid ports, and reserved hostnames.

## Abuse resistance

**Pass in implementation**

- PostgreSQL-backed fixed-window rate limiting exists for IP, account, and API-key principals.
- IP rate-limit keys are salted hashes rather than durable raw IP strings.
- Public abuse reporting, internal review, URL rules, link suspension, and audit history are implemented.
- Gate 6 publishes a no-account `/report-abuse` form backed by the existing rate-limited abuse-report API.
- Production requires a strong admin token and rate-limit salt.

**Blocker — launch-environment verification**

- Submit a real launch-environment abuse report and complete review → suspension → redirect blocked → resolution.
- Confirm proxy source-IP configuration produces the real client bucket rather than the Caddy container address.

## Browser, HTTP, TLS, and custom-domain security

**Pass**

- Session cookies are HTTP-only and production cookies require HTTPS.
- The application sets content-type, frame, referrer, permissions, and CSP response headers.
- Production base URL is required to use HTTPS.
- Caddy is the only public-facing service in the reference deployment.
- Internal metrics, readiness, and TLS-authorization endpoints are blocked by the public proxy.
- On-demand certificate issuance uses qh8z's internal `ask` endpoint rather than approving arbitrary hostnames.
- Every custom-domain request is authorized through the same verified-domain/paid-entitlement path.
- Automated tests cover Free → denied, active Pro → allowed, past-due Pro grace → allowed, and canceled Pro → denied.
- Gate 5 CI validated the exact Caddy configuration that contains the per-request authorization rule.

## Secrets and providers

**Pass in implementation**

- qh8z runs non-root in production.
- File-mounted secrets use the documented `root:65532` / `0640` model rather than world-readable files.
- Docker build context excludes `.ci-secrets`, environment files, secret directories, and local state.
- Production startup refuses missing PostgreSQL, SMTP, Web Risk, admin/rate-limit, or Stripe requirements.
- Stripe webhook signatures are verified against the raw request body with timestamp tolerance.
- Stripe webhook event IDs are durably claimed for idempotency and released after failed processing so the provider can retry.

**Blocker — launch-environment verification**

- Scan current tree and Git history for real production credentials immediately before launch.
- Verify provider keys use the minimum practical permissions and can be independently rotated.
- Verify the Stripe production webhook points to the exact qh8z production endpoint and rejects an invalid signature.

## Billing and entitlement security

**Pass**

- Free/Pro resource limits are enforced server-side.
- Billing management requires an owner/admin browser session.
- Custom-domain creation requires Pro.
- Canceled Pro workspaces fall back to Free entitlements.
- Gate 5 added per-request custom-domain entitlement enforcement so a cached certificate cannot preserve branded traffic after cancellation.
- Memory and PostgreSQL tests cover the paid-domain entitlement transition.

**Blocker — launch-environment verification**

- Complete one real Stripe test/live-mode subscription flow on the launch environment: checkout → webhook → Pro entitlement → customer portal → cancel → webhook → branded-domain denial.

## Data protection and recovery

**Pass**

- PostgreSQL is private to the internal production network.
- Production backups use `pg_dump` plus encrypted restic storage.
- Restore is guarded by an explicit confirmation value.
- Backup retention, deployment backups, restore drills, rollback, readiness, and failure procedures are documented.
- Gate 5 exact-head CI created an encrypted backup, restored it into a fresh database, verified the seeded link, and proved readiness failure/recovery through a deliberate PostgreSQL outage.

## Logging and privacy

**Pass in design/implementation**

- qh8z application logs use structured JSON in production.
- Caddy access logs are configured as JSON with bounded Docker log rotation.
- Prometheus metrics do not label metrics with destination URLs, user emails, API secrets, or IP addresses.
- Visitor IPs are not required for product analytics and durable rate-limit IP keys are hashed.

**Blocker — launch-environment verification**

- Inspect representative application, Caddy, PostgreSQL, and alert output and confirm passwords, session tokens, API-key secrets, Stripe secrets, Web Risk keys, and email-verification tokens are not logged.

## Dependency and build security

**Pass for current build controls**

- Go module files are locked and CI rejects `go mod tidy` drift.
- CI runs formatting, `go vet`, race-enabled tests, production build, Compose validation, production proxy/monitoring validation, container builds, backup/restore, load, and failure recovery.
- The application runtime image is `scratch`, runs as a non-root numeric user, and includes only the qh8z and healthcheck binaries.
- The production Compose configuration drops application capabilities, sets `no-new-privileges`, and uses a read-only qh8z filesystem.

**Blocker — exact launch SHA review**

- Review all direct/indirect Go dependencies and production container image versions for known high-severity vulnerabilities immediately before launch.
- Record the exact launch Git SHA and production image identifiers.

## Security-reporting process

**Implemented in repository / Gate 6**

- `SECURITY.md` defines private vulnerability-reporting and safe-research expectations.
- `/security` publishes that source-of-truth policy from the application binary.
- `/.well-known/security.txt` publishes the canonical security contact and policy URL.

**Blocker — live contact verification**

- `security@qh8z.com` must accept external mail and be monitored before public launch.
- Do not ask researchers to publish exploit details before a fix is available.
- Preserve a timeline for confirmed incidents and rotate exposed credentials immediately.

## Open launch blockers from this review

1. Cross-workspace and API-key-scope negative authorization matrix on the launch candidate.
2. Production unsafe-destination matrix on the launch binary/proxy.
3. Production secret/current-tree/history scan and provider least-privilege review.
4. Real Stripe webhook/subscription/cancellation entitlement test.
5. Representative production log inspection for credential/token leakage.
6. Dependency/container vulnerability review recorded for the exact launch SHA.
7. Live abuse report → suspension → resolution exercise and source-IP verification.
8. `security@qh8z.com` external delivery/monitoring verification.

The Issue #3 **Security review** checkbox stays open until every blocker above is closed or explicitly waived with a documented, risk-accepted reason.