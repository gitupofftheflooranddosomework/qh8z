# QH8Z

**Short links. Long lifespan.**

QH8Z is a commercial link-shortening platform being built around `qh8z.com`.

The first implementation uses **Shlink** as the replaceable redirect/link engine and treats **Kutt** as a product/reference donor for features we selectively adapt. QH8Z owns the customer-facing product layer, authentication, billing, abuse controls, and future business logic.

## Current status

Early bootstrap. This branch establishes the architecture, local Shlink stack, and a small QH8Z gateway API.

- Shlink engine pinned to `v5.1.5`
- Kutt reference baseline `v3.2.6`
- PostgreSQL-backed local development
- QH8Z gateway with authenticated link creation
- Anonymous public link creation intentionally disabled
- Third-party license notices tracked from day one

## Architecture

```text
Users
  |
  +--> qh8z.com/<slug> ----------> Shlink redirect engine
  |
  +--> app.qh8z.com ------------> QH8Z product UI (planned)
                                   |
                                   +--> QH8Z API / gateway
                                          |
                                          +--> Shlink REST API
                                          +--> QH8Z accounts/billing DB (planned)
```

Shlink is intentionally behind a boundary so it can be replaced later without rebuilding the QH8Z customer/business layer.

Kutt will **not** be merged wholesale into the same runtime. We will selectively port or adapt useful MIT-licensed product code/patterns where they improve QH8Z and record the origin of any substantial reused code.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the design.

## Local development

Requirements: Docker with Compose.

```bash
cp .env.example .env
# Replace the placeholder secrets in .env
docker compose up --build
```

Local services:

- Shlink redirect/API: `http://localhost:8080`
- QH8Z gateway: `http://localhost:3000`
- PostgreSQL: internal Docker network only

Health check:

```bash
curl http://localhost:3000/healthz
```

Create a link through the QH8Z gateway:

```bash
curl -X POST http://localhost:3000/api/links \
  -H "Authorization: Bearer $QH8Z_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"longUrl":"https://example.com","customSlug":"example"}'
```

In local development the returned short URL is served by Shlink on port `8080`. Production routing will place Shlink behind `qh8z.com` while the product UI/API live on separate hosts.

## Product principles

1. **No anonymous public shortening at launch.** Protect the domain reputation first.
2. **Keep the redirect engine replaceable.** QH8Z should remain valuable even if we replace Shlink later.
3. **Own the business layer.** Accounts, plans, billing, teams, abuse controls, and premium features belong to QH8Z.
4. **Use permissive OSS surgically.** Preserve license notices and provenance for reused code.
5. **Build a saleable asset.** Keep IP boundaries and third-party attribution clear from the beginning.

## Licensing

QH8Z does not currently declare a project-wide open-source license. Third-party components retain their respective licenses and notices. See [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) and [`licenses/`](licenses/).
