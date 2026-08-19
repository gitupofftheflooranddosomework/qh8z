# qh8z

**qh8z** is a modern URL shortener being built as an original commercial-ready product.

Rather than literally merging Kutt's JavaScript codebase with Shlink's PHP codebase, qh8z takes inspiration from strong ideas in both while keeping one coherent architecture that we own and can evolve.

## Current milestone

The qh8z launch candidate includes:

- PostgreSQL-backed durable short links and visit analytics
- embedded, idempotent database migrations
- account registration/login/logout and verified email
- workspaces with owner/admin/member roles
- workspace-owned links and scoped API keys
- full link management, QR codes, custom domains, and DNS verification
- dashboard analytics with plan-based retention
- Free and Pro entitlements, usage metering, Stripe checkout, webhooks, and customer portal
- distributed per-IP/user/API-key rate limits
- Google Web Risk reputation checks, managed allow/block rules, abuse reporting, and suspension/review tooling
- secure session cookies and hashed session/verification/API-key secrets
- production Caddy/TLS, PostgreSQL, Prometheus, Alertmanager, structured logs, readiness/liveness, alerts, and offsite-backup tooling
- guarded deployment, encrypted backup/restore, rollback, load, and failure-test procedures
- public Terms, Privacy Policy, Acceptable Use Policy, pricing, abuse-reporting, and security-reporting pages
- GitHub Actions coverage for race tests, the full customer/billing journey, secret-history scanning, reachable Go vulnerabilities, production-image vulnerability scanning, backup/restore, redirect load, and failure recovery

> **Launch status:** repository-side product, safety, commercial, operations, and launch/legal work is implemented through GitHub Issue #3. Public launch still requires the live-environment verification documented in [`docs/LAUNCH_RUNBOOK.md`](docs/LAUNCH_RUNBOOK.md), including production DNS/TLS, provider credentials and Stripe flow, monitored role mailboxes, production abuse/log checks, and the final qh8z.com smoke test. Issue #3 remains the authoritative launch gate until those checks pass.

## Run locally with PostgreSQL

```bash
docker compose up --build
```

Open `http://localhost:8080`.

For a local in-memory development process:

```bash
QH8Z_STORAGE=memory QH8Z_EMAIL_MODE=log go run ./cmd/qh8z
```

### Core environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `QH8Z_BASE_URL` | `http://localhost:$PORT` | Public base URL and verification-link origin |
| `QH8Z_ENV` | `development` | `production` enables production startup requirements |
| `QH8Z_STORAGE` | `memory` | `memory` or `postgres`; production requires `postgres` |
| `DATABASE_URL` | none | PostgreSQL connection string |
| `QH8Z_EMAIL_MODE` | `log` | `log` or `smtp`; production requires `smtp` |

SMTP settings are documented in [`docs/IDENTITY.md`](docs/IDENTITY.md).

## Authentication API

Register:

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'content-type: application/json' \
  -d '{"email":"owner@example.com","password":"correct horse battery staple","workspaceName":"My Team"}'
```

After email verification, session-cookie requests can create workspace-owned links. Non-browser clients can use scoped `qh8z_sk_...` API keys.

Use `X-QH8Z-Workspace` to choose among multiple workspaces.

## Link API

Create a link with an authenticated, verified session or an API key with `links:write`:

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer qh8z_sk_REDACTED' \
  -d '{"url":"https://example.com/some/long/path","customSlug":"example"}'
```

Read a managed link with `links:read`:

```bash
curl -H 'Authorization: Bearer qh8z_sk_REDACTED' http://localhost:8080/api/v1/links/example
```

Read stats with `analytics:read`:

```bash
curl -H 'Authorization: Bearer qh8z_sk_REDACTED' http://localhost:8080/api/v1/links/example/stats
```

The public redirect remains:

```text
http://localhost:8080/example
```

## Operations and architecture

- [`docs/IDENTITY.md`](docs/IDENTITY.md) — accounts, workspaces, verification, API keys, and audit behavior.
- [`docs/SAFETY.md`](docs/SAFETY.md) — rate limits, Web Risk, destination validation, abuse reports, and admin safety controls.
- [`docs/BACKUP_RESTORE.md`](docs/BACKUP_RESTORE.md) — PostgreSQL backup and recovery procedure.
- [`docs/CACHING.md`](docs/CACHING.md) — Redis-compatible redirect caching strategy.
- [`docs/PRODUCTION_OPERATIONS.md`](docs/PRODUCTION_OPERATIONS.md) — production deployment, monitoring, backup, restore, rollback, and failure operations.
- [`docs/LAUNCH_RUNBOOK.md`](docs/LAUNCH_RUNBOOK.md) — final live-environment launch verification.
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — product roadmap beyond the launch gate.
- GitHub Issue #3 is the authoritative public-launch gate.

## Open-source inspiration

qh8z is informed by projects such as Kutt and Shlink. The qh8z implementation is original code rather than copied source from either project.

## License

MIT.
