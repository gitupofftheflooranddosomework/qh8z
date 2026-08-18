# qh8z

**qh8z** is a modern URL shortener being built as an original commercial-ready product.

Rather than literally merging Kutt's JavaScript codebase with Shlink's PHP codebase, qh8z takes inspiration from the strongest ideas in both while keeping one coherent architecture that we own and can evolve.

## Current milestone

The first qh8z core includes:

- JSON API for creating short links
- Generated 7-character slugs
- Optional custom slugs
- HTTP redirects
- Basic per-link visit counts
- Minimal browser UI
- URL validation and reserved slugs
- Security response headers
- Health endpoint
- Dependency-free Go implementation
- Docker and GitHub Actions CI

> **Development status:** storage is currently in-memory. Links and counters reset whenever the process restarts. Authentication, PostgreSQL, abuse controls, rate limiting, custom domains, richer analytics, and billing are required before production launch.

## Run locally

Requires Go 1.23+.

```bash
go run ./cmd/qh8z
```

Open `http://localhost:8080`.

Environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `QH8Z_BASE_URL` | `http://localhost:$PORT` | Public base URL returned for short links |

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

Redirect:

```text
http://localhost:8080/example
```

## Direction

Next: PostgreSQL persistence, accounts/workspaces, custom domains, production analytics, abuse controls, API keys, and paid-plan entitlements. See [`docs/ROADMAP.md`](docs/ROADMAP.md).

## Open-source inspiration

qh8z is informed by projects such as Kutt and Shlink. Both currently use the MIT License. The initial qh8z implementation is original code rather than copied source from either project.

## License

MIT.
