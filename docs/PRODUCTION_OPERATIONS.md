# qh8z production operations runbook

This runbook is the operational contract for Issue #3 Gate 5. Commands assume the repository lives at `/opt/qh8z` and the production environment file is `deploy/.env.production`.

Define the Compose command once per shell if desired:

```bash
alias qh8z-compose='docker compose --env-file deploy/.env.production -f deploy/compose.production.yml'
```

## Normal deployment

1. Confirm the branch/commit intended for production and record the current production SHA.
2. Confirm CI is green for the candidate SHA.
3. Confirm all secret files exist under `QH8Z_SECRETS_DIR`, are owned by `root:${QH8Z_SECRET_GID:-65532}`, and are mode `0640`; the directory itself must be mode `0750`.
4. Validate the Compose configuration.
5. Take a one-shot encrypted offsite database backup.
6. Build the qh8z and backup images.
7. Start/update the stack.
8. Wait for qh8z to become healthy.
9. Check the public liveness endpoint and Prometheus targets.

Example:

```bash
git rev-parse HEAD
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml config --quiet
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml run --rm -e BACKUP_ONCE=1 backup
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml up -d --build --remove-orphans
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml ps
curl -fsS https://qh8z.com/healthz
```

Do not deploy an unreviewed local working tree. Production should run a Git commit whose CI result can be traced.

## Health and readiness

`GET /healthz` is liveness: the process can serve HTTP.

`GET /readyz` is readiness: qh8z can reach its storage backend. Caddy does not expose `/readyz` publicly, but the qh8z Docker health check invokes it from inside the container.

`GET /metrics` is Prometheus-format telemetry and is also blocked by Caddy from public access.

Useful commands:

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml ps
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml exec -T qh8z /healthcheck
curl -fsS https://qh8z.com/healthz
```

## Alert response

### Qh8zDown

1. Check Caddy and qh8z container state.
2. Inspect recent logs.
3. Check host disk, memory, and Docker daemon state.
4. If qh8z is unhealthy but PostgreSQL is healthy, restart qh8z only.
5. If the new release introduced the failure, follow the rollback procedure.

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml ps
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml logs --since 20m caddy qh8z
```

### Qh8zStorageDown

1. Inspect PostgreSQL health and logs.
2. Check disk capacity before restarting PostgreSQL.
3. Do not delete the database volume as a troubleshooting step.
4. If PostgreSQL data is corrupt or missing, move to the restore procedure.

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml ps postgres
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml logs --since 30m postgres
```

### Qh8zHighErrorRate

1. Check qh8z logs for repeated request/storage/provider errors.
2. Inspect Prometheus request and storage metrics.
3. Check Stripe, Web Risk, SMTP, and PostgreSQL connectivity as appropriate to the failing endpoint.
4. Roll back if the error rate correlates with a deployment.

### Qh8zRateLimitSpike

1. Confirm whether traffic is legitimate or abusive.
2. Inspect source patterns without logging raw user credentials or sensitive tokens.
3. Use existing block/suspension tooling for abuse; do not disable rate limiting globally as the first response.

## Rollback

The application migrations are designed to be additive and forward-compatible. A routine application rollback therefore keeps the existing PostgreSQL volume and schema.

1. Record the failed SHA.
2. Identify the last known-good SHA with green CI.
3. Checkout that exact SHA on the production host.
4. Rebuild and replace only the qh8z container first.
5. Verify readiness and the public health endpoint.
6. If the incident involved proxy/monitoring configuration, restore those services from the known-good SHA too.

```bash
git checkout --detach <known-good-sha>
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml build qh8z
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml up -d --no-deps qh8z
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml exec -T qh8z /healthcheck
curl -fsS https://qh8z.com/healthz
```

If a future migration is intentionally destructive or incompatible with the previous application, that migration must ship with an explicit migration-specific rollback plan before it can be deployed. Do not blindly restore the whole database to roll back an ordinary application bug.

## Backup verification

The backup service performs encrypted restic backups on a six-hour default interval. Confirm snapshots periodically:

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml run --rm --entrypoint sh backup -c '
export RESTIC_PASSWORD_FILE=/run/secrets/restic_password
export AWS_ACCESS_KEY_ID="$(cat /run/secrets/restic_s3_access_key)"
export AWS_SECRET_ACCESS_KEY="$(cat /run/secrets/restic_s3_secret_key)"
restic snapshots --tag qh8z-postgres
'
```

A backup is not considered operationally proven until a restore drill succeeds against a disposable database.

## Restore drill / disaster restore

Prefer a disposable PostgreSQL instance for drills. For a real production restore, place qh8z in maintenance/offline mode first so writes cannot race the restore.

List snapshots, choose the desired snapshot ID, then run the guarded restore command. `CONFIRM_RESTORE` must exactly match the database name or the script refuses to run.

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml stop qh8z

docker compose --env-file deploy/.env.production -f deploy/compose.production.yml run --rm \
  --entrypoint /usr/local/bin/restore.sh \
  -e RESTORE_SNAPSHOT=<snapshot-id-or-latest> \
  -e CONFIRM_RESTORE=qh8z \
  backup

docker compose --env-file deploy/.env.production -f deploy/compose.production.yml start qh8z
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml exec -T qh8z /healthcheck
```

After a disaster restore, validate account login, link lookup, redirects, analytics, custom domains, and billing state before reopening normal traffic.

## Load test

Run load tests against a staging short link before launch and after material redirect-path changes. The tool does not follow redirects, so it measures the qh8z redirect response itself.

Example launch threshold:

```bash
go run ./cmd/loadtest \
  -url https://staging.qh8z.com/<known-short-code> \
  -duration 60s \
  -concurrency 50 \
  -expect-status 302 \
  -max-error-rate 0.01 \
  -max-p95 500ms
```

The launch gate requires the chosen staging/production-sized host to pass the recorded threshold without PostgreSQL errors, sustained 5xx alerts, or an unhealthy qh8z container.

## Controlled failure test

`deploy/failure-test.sh` deliberately stops PostgreSQL and restarts qh8z. Run it only on staging or during an announced maintenance window.

```bash
QH8Z_FAILURE_TEST_ACK=maintenance-window \
QH8Z_ENV_FILE=deploy/.env.production \
deploy/failure-test.sh
```

The test passes only if:

- qh8z is initially ready,
- readiness fails when PostgreSQL is stopped,
- readiness recovers after PostgreSQL returns,
- qh8z becomes ready after an application restart.

## TLS and custom-domain incidents

The primary domain is managed directly by Caddy. Customer domains use on-demand TLS with an internal authorization request to `/internal/tls/allow?domain=<host>`.

That endpoint returns success only when the hostname exists as a verified custom domain and its workspace currently has Pro entitlement. Caddy uses the check both for on-demand certificate issuance and before every custom-domain request, so a canceled Pro workspace stops serving branded links even if Caddy still has a cached certificate. The current launch grace policy keeps Pro entitlement during `past_due` and removes it when billing status becomes `canceled`.

Caddy blocks `/internal/*` from public access and qh8z itself is not published on a host port in production.

If certificate issuance or branded traffic fails:

1. confirm the customer's A/AAAA/CNAME reaches the qh8z server,
2. confirm their qh8z TXT ownership verification is complete,
3. confirm the domain appears verified in the dashboard,
4. confirm the workspace still has Pro entitlement,
5. inspect Caddy logs for ACME, authorization, or rate-limit messages,
6. never change the TLS authorization endpoint to approve arbitrary hostnames as a workaround.

## Secrets incident

If any production secret is exposed:

1. rotate it at the provider immediately,
2. replace the corresponding file under `/etc/qh8z/secrets`,
3. rerun `deploy/init-secrets.sh` to restore `root:${QH8Z_SECRET_GID:-65532}` ownership and mode `0640`,
4. restart only the services that consume that secret,
5. revoke old Stripe/Web Risk/SMTP/object-storage credentials at their provider,
6. review access and application logs for abuse,
7. rotate qh8z admin token/rate-limit salt as appropriate.

Secret file contents must never be pasted into GitHub issues, application logs, or the repository.
