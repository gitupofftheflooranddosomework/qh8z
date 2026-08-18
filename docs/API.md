# QH8Z Developer API

QH8Z Pro accounts can create scoped API tokens from **Dashboard → Developer**. The raw token is shown once and is never stored by QH8Z; only its SHA-256 hash and display prefix are retained.

## Authentication

Send the token as a bearer credential:

```http
Authorization: Bearer qh8z_live_...
```

The v1 API lives under:

```text
https://qh8z.com/api/v1
```

Scopes:

- `links:read` — read link inventory and statistics.
- `links:write` — create, edit, disable, and restore links. Write endpoints also require the read scope when they return managed link records.

A revoked, expired, unknown, or out-of-scope token is rejected. Suspended accounts and accounts no longer eligible for QH8Z management cannot use write operations.

## Identify the token account

```bash
curl https://qh8z.com/api/v1/me \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

## List links

```bash
curl 'https://qh8z.com/api/v1/links?status=active&limit=25&offset=0&q=launch&tag=campaign' \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

Supported query parameters:

| Parameter | Meaning |
|---|---|
| `limit` | 1–100 records per page; default 25 |
| `offset` | zero-based pagination offset |
| `q` | search short code, destination, title, and private notes |
| `status` | `all`, `active`, `disabled`, `archived`, or `expired` |
| `tag` | exact normalized tag filter |

Response shape:

```json
{
  "links": [],
  "total": 0,
  "limit": 25,
  "offset": 0,
  "hasMore": false
}
```

## Create a link

```bash
curl -X POST https://qh8z.com/api/v1/links \
  -H "Authorization: Bearer $QH8Z_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "longUrl": "https://example.com/campaign",
    "customSlug": "launch",
    "title": "Launch campaign",
    "tags": ["campaign", "launch"],
    "notes": "Internal note",
    "expiresAt": "2026-12-01T00:00:00.000Z",
    "maxVisits": 5000
  }'
```

`customSlug`, `title`, `tags`, and `notes` are optional. `expiresAt` and `maxVisits` are Pro redirect controls.

Tags are lowercase, deduplicated, and limited to 12 tags of up to 32 characters each. Private notes are limited to 2,000 characters. Expiration must be at least one minute in the future and no more than five years away. Maximum visits must be an integer from 1 to 10,000,000.

All destinations pass QH8Z URL policy and, in public-launch mode, DNS/private-network checks and URL reputation screening before Shlink is mutated.

## Get one link

```bash
curl https://qh8z.com/api/v1/links/LINK_ID \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

## Link statistics

```bash
curl https://qh8z.com/api/v1/links/LINK_ID/stats \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

Statistics include the managed short URL, destination, title, tags, configured expiration/max-visits controls, and Shlink's visit summary.

## Update a link

```bash
curl -X PATCH https://qh8z.com/api/v1/links/LINK_ID \
  -H "Authorization: Bearer $QH8Z_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "longUrl": "https://example.com/new-destination",
    "title": "Updated campaign",
    "tags": ["campaign", "updated"],
    "notes": "Destination changed after launch"
  }'
```

Omitted fields keep their existing values. Disabled links must be restored before they can be edited.

## Disable a link

```bash
curl -X DELETE https://qh8z.com/api/v1/links/LINK_ID \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

Disabling removes the redirect from Shlink but retains QH8Z ownership/history so the short code cannot be claimed by another account.

## Restore a disabled link

```bash
curl -X POST https://qh8z.com/api/v1/links/LINK_ID/restore \
  -H "Authorization: Bearer $QH8Z_TOKEN"
```

Restore reclaims the same QH8Z-owned short code. Alias ownership is checked in PostgreSQL before Shlink is touched, and only the exact owning link record can use the restore path.

## Browser-only product endpoints

The session-authenticated dashboard also exposes operations that are intentionally not part of the first public bearer API contract, including bulk creation, CSV export, archive/unarchive, QR rendering, visit-history pages, billing, account settings, and trust/safety administration. These may be promoted into future `/api/v1` endpoints when their automation contracts are stable.

## Errors

Errors use an HTTP status and JSON body such as:

```json
{
  "error": "feature_requires_pro",
  "message": "Expiry, max-visit controls, bulk creation, and developer API access require QH8Z Pro."
}
```

Common classes include `400` validation errors, `401` invalid/expired API tokens, `402` plan/feature limits, `403` account eligibility or scope failures, `404` unknown owned resources, `409` alias/state conflicts, `422` unsafe destinations, and `503` required dependency failures.

Clients should treat undocumented 5xx responses as transient and use bounded retry/backoff rather than assuming a mutation did not occur. QH8Z itself keeps durable ownership/reconciliation state to resolve ambiguous Shlink operations.
