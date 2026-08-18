# Redirect-path caching strategy

PostgreSQL is the source of truth for links. A Redis-compatible cache will be a disposable acceleration layer and must never be required to recover link data.

## Cache model

- Key: `redirect:v1:{host}:{slug}`. Including host keeps the design compatible with future custom domains.
- Positive value: destination URL plus a link version/status field once editable links are introduced.
- Positive TTL: start at 5 minutes and tune from production hit-rate and update-frequency data.
- Negative cache: missing/disabled links may be cached for no more than 30 seconds to reduce repeated database misses without making newly-created links appear unavailable for long.

## Read path

1. Resolve `{host, slug}` from the request.
2. Read Redis.
3. On a hit, redirect immediately.
4. On a miss or cache error, read PostgreSQL.
5. Populate Redis after a successful database read.
6. If Redis is unavailable, fail open to PostgreSQL. A cache outage must not take redirects offline.

Analytics events and counters are not served from this cache. The cache is only for link resolution.

## Invalidation

Evict the exact redirect key after link create/update/disable/delete and after any custom-domain ownership/status change. When link versioning is added, write paths should update PostgreSQL first and invalidate cache only after the transaction commits.

## Stampede and abuse controls

Use a short per-process singleflight/coalescing window for simultaneous misses on the same key. Apply rate limits before expensive reputation/analytics work. Do not place attacker-controlled unbounded payloads in Redis.

## Deployment order

Redis support should be added after durable PostgreSQL behavior is proven. The application must continue to operate correctly with Redis completely disabled; this keeps recovery simple and avoids cache/data divergence becoming a launch blocker.
