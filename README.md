# qh8z

**qh8z** is a modern URL shortener being built as an original commercial-ready product.

Rather than literally merging Kutt's JavaScript codebase with Shlink's PHP codebase, qh8z takes inspiration from strong ideas in both while keeping one coherent architecture that we own and can evolve.

## Current milestone

The qh8z core now includes:

- JSON API for creating short links
- Generated 7-character slugs and optional custom slugs
- HTTP redirects
- PostgreSQL-backed durable links
- Durable visit events and counters
- Per-link stats endpoint
- Embedded, idempotent database migrations
- Readiness and liveness endpoints
- Safe development-only in-memory storage
- Production guard that refuses in-memory storage
- Structured JSON logging and graceful shutdown
- Minimal browser UI
- URL validation and reserved slugs
- Security response headers and HTTP timeouts
- Docker, Compose, and PostgreSQL-backed GitHub Actions CI

> **Development status:** durable storage is in place, but authentication, abuse controls, custom domains, richer analytics, billing, production deployment, legal policies, and the remaining launch gates are still required before public launch.

## Run locally with PostgreSQL

Requires Docker Compose, or Go 1.25+ plus PostgreSQL.

```bash
docker compose up --build
```

Open `http://localhost:8080`.

For a local in-memory development process:

```bash
QH8Z_STORAGE=memory go run ./cmd/qh8z
```

Environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `QH8Z_BASE_URL` | `http://localhost:$PORT` | Public base URL returned for short links |
| `QH8Z_ENV` | `development` | Set to `production` to enable production safety checks |
| `QH8Z_STORAGE` | `memory` | `memory` or `postgres`; production requires `postgres` |
| `DATABASE_URL` | none | PostgreSQL connection string when storage is `postgres` |

Database migrations run automatically on PostgreSQL startup and are recorded in `schema_migrations`.

## API

Create a link:

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com/some/long/path","customSlug":"example"}'
```

Read it:

```bash
curl http://localhost:8080/api/v1/links/example
```

Read stats:

```bash
curl http://localhost:8080/api/v1/links/example/stats
```

Redirect:

```text
http://localhost:8080/example
```

Health endpoints:

- `/healthz` confirms the process is alive.
- `/readyz` confirms the configured storage is reachable.

## Operations

- [`docs/BACKUP_RESTORE.md`](docs/BACKUP_RESTORE.md) — manual backup and recovery procedure.
- [`docs/CACHING.md`](docs/CACHING.md) — Redis-compatible redirect caching strategy.
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — product/launch roadmap.
- GitHub Issue #3 is the authoritative public-launch gate.

## Open-source inspiration

qh8z is informed by projects such as Kutt and Shlink. The qh8z implementation is original code rather than copied source from either project.

## License

MIT.
