# qh8z launch security review

**Review date: August 18, 2026**

This is the launch security review for Issue #3. It is an engineering review of the qh8z implementation and deployment controls; it is not a claim of third-party certification or a substitute for an independent penetration test.

## Release rule

A finding marked **Blocker** must be closed before the Issue #3 security-review checkbox can be marked complete. Medium/low post-launch improvements may remain only when their risk, owner, and mitigation are documented.

## Identity and authorization

**Pass**

- Browser sessions and API keys are separate credential types.
- Links are workspace-owned rather than globally editable by any authenticated account.
- API keys are scoped and store only a hash of the issued secret.
- Session/verification secrets are stored as hashes rather than plaintext tokens.
- Email verification is required before link creation.
- Workspace owner/admin checks protect billing and workspace-administration operations.
- Audit records exist for sensitive workspace, link, API-key, billing, and admin actions.

**Launch verification required**

- Attempt cross-workspace reads/writes for links, domains, analytics, members, API keys, audit, and billing and confirm `403`/`404` as appropriate.
- Attempt every API-key scope boundary with a key missing the required scope.
- Confirm logout invalidates the active browser session.

## Destination and redirect security

**Pass**

- Only HTTP/HTTPS destinations are accepted.
- URLs containing credentials are rejected.
- Localhost, private, link-local, reserved, and numeric-IP-like destinations are rejected to reduce SSRF/open-redirect abuse against internal services.
- URL reputation checks are required in production and failures fail closed for new link creation.
- Managed block/allow rules exist; static private/local destination protections are not bypassed by an allow rule.
- Link edits repeat destination/reputation validation rather than trusting a previously-created link.
- Suspended/disabled links are removed from public redirects.

**Launch verification required**

- Repeat the unsafe-destination test matrix against the production binary and proxy, including IPv4, IPv6, encoded/numeric host forms, credentials, invalid ports, and reserved hostnames.

## Abuse resistance

**Pass**

- PostgreSQL-backed fixed-window rate limiting exists for IP, account, and API-key principals.
- IP rate-limit keys are salted hashes rather than durable raw IP strings.
- Public abuse reporting, internal review, URL rules, link suspension, and audit history are implemented.
- Production requires a strong admin token and rate-limit salt.

**Launch verification required**

- Submit a real launch-environment abuse report and complete review → suspension → redirect blocked → resolution.
- Confirm proxy source-IP configuration produces the real client bucket rather than the Caddy container address.

## Browser and HTTP security

**Pass**

- Session cookies are HTTP-only and production cookies require HTTPS.
- The application sets content-type, frame, referrer, permissions, and CSP response headers.
- Production base URL is required to use HTTPS.
- Caddy is the only public-facing service in the reference deployment.
- Internal metrics, readiness, and TLS-authorization endpoints are blocked by the public proxy.

**Blocker until Gate 5 is green**

- The exact production Caddy configuration must pass automated validation and the full production rehearsal on the same commit.

## Secrets and providers

**Pass in implementation**

- Production credentials can be mounted from files rather than committed or exposed as ordinary application environment values.
- Production startup refuses missing PostgreSQL, SMTP, Web Risk, admin/rate-limit, or Stripe requirements.
- Stripe webhook signatures are verified against the raw request body with timestamp tolerance.
- Stripe webhook event IDs are durably claimed for idempotency and released after failed processing so the provider can retry.

**Launch verification required**

- Scan the Git history and current tree for real production credentials before launch.
- Verify provider keys use the minimum practical permissions and can be independently rotated.
- Verify Stripe production webhook points to the exact qh8z production endpoint and rejects an invalid signature.

## Billing and entitlement security

**Pass in application layer**

- Free/Pro resource limits are enforced server-side.
- Billing management requires an owner/admin browser session.
- Custom-domain creation requires Pro.
- Canceled Pro workspaces fall back to Free entitlements.

**Blocker until Gate 5 validation completes**

- Verified custom-domain TLS authorization and request routing must be tested after a Pro → canceled transition so cached certificates cannot preserve a paid feature unintentionally.

## Data protection and recovery

**Pass in implementation**

- PostgreSQL is private to the internal production network.
- Production backups use `pg_dump` plus encrypted restic storage.
- Restore is guarded by an explicit confirmation value.
- Backup retention, deployment backups, restore drills, rollback, readiness, and failure procedures are documented.

**Blocker until Gate 5 rehearsal is green**

- CI/staging must create an encrypted backup, restore into a fresh database, verify the seeded link, and prove readiness fails/recover when PostgreSQL is deliberately stopped/restarted.

## Logging and privacy

**Pass with launch checks**

- qh8z application logs use structured JSON in production.
- Caddy access logs are configured as JSON with bounded Docker log rotation.
- Prometheus metrics do not label metrics with destination URLs, user emails, API secrets, or IP addresses.
- Visitor IPs are not required for product analytics and durable rate-limit IP keys are hashed.

**Launch verification required**

- Inspect representative application, Caddy, PostgreSQL, and alert output and confirm passwords, session tokens, API-key secrets, Stripe secrets, Web Risk keys, and email-verification tokens are not logged.

## Dependency and build security

**Pass for current build controls**

- Go module files are locked and CI rejects `go mod tidy` drift.
- CI runs formatting, `go vet`, race-enabled tests, production build, Compose validation, and container builds.
- The application runtime image is `scratch`, runs as a non-root numeric user, and includes only the qh8z and healthcheck binaries.
- The production Compose configuration drops application capabilities, sets `no-new-privileges`, and uses a read-only qh8z filesystem.

**Launch verification required**

- Review all direct/indirect Go dependencies and production container image versions for known high-severity vulnerabilities immediately before launch.
- Record the exact launch Git SHA and image identifiers.

## Security-reporting process

**Required before launch**

- Publish `SECURITY.md` with supported-version and reporting instructions.
- `security@qh8z.com` must accept external mail and be monitored.
- Do not ask researchers to publish exploit details before a fix is available.
- Preserve a timeline for confirmed incidents and rotate exposed credentials immediately.

## Open launch blockers from this review

1. Gate 5 production proxy/backup/load/failure rehearsal must be fully green.
2. Cross-workspace/scope negative authorization matrix must pass against the launch candidate.
3. Production secret/history scan must be clean.
4. Production custom-domain Pro → canceled test must deny branded traffic.
5. Production Stripe invalid/valid webhook test must pass.
6. Representative logs must be inspected for credential/token leakage.
7. Dependency/container vulnerability review must be recorded for the exact launch SHA.
8. The public `security@qh8z.com` reporting channel must be live.

The Issue #3 **Security review** checkbox stays open until every blocker above is closed or explicitly waived with a documented, risk-accepted reason.