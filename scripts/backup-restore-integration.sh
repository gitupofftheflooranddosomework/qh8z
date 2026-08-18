#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  docker compose --profile production logs --no-color > /tmp/qh8z-backup-restore-compose.log 2>&1 || true
  docker compose --profile production down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

assert_db_isolation() {
  [[ "$(docker compose exec -T -e PGPASSWORD="$QH8Z_DB_PASSWORD" db psql -h 127.0.0.1 -U qh8z_app -d qh8z -Atc 'SELECT 1')" == "1" ]]
  [[ "$(docker compose exec -T -e PGPASSWORD="$SHLINK_DB_PASSWORD" db psql -h 127.0.0.1 -U shlink_app -d shlink -Atc 'SELECT 1')" == "1" ]]
  if docker compose exec -T -e PGPASSWORD="$QH8Z_DB_PASSWORD" db psql -h 127.0.0.1 -U qh8z_app -d shlink -Atc 'SELECT 1' >/tmp/qh8z-cross-db-1.log 2>&1; then
    echo 'qh8z_app unexpectedly connected to the Shlink database' >&2
    exit 1
  fi
  if docker compose exec -T -e PGPASSWORD="$SHLINK_DB_PASSWORD" db psql -h 127.0.0.1 -U shlink_app -d qh8z -Atc 'SELECT 1' >/tmp/qh8z-cross-db-2.log 2>&1; then
    echo 'shlink_app unexpectedly connected to the QH8Z database' >&2
    exit 1
  fi
}

docker compose --profile production up -d --build --remove-orphans
ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1 && curl -kfsS https://localhost/healthz >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]]
assert_db_isolation

# Simulate an ambiguous create handoff: Shlink has a live redirect, QH8Z has no
# ownership row, but the pre-mutation create intent survived. Reconciliation
# must remove the unclaimed redirect and the journal entry.
curl -fsS -X POST http://localhost:8080/rest/v3/short-urls \
  -H "X-Api-Key: ${SHLINK_API_KEY}" -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/orphan","customSlug":"orphan-ci"}' >/tmp/qh8z-orphan-link.json
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 -c "INSERT INTO shlink_create_intents(short_code,long_url,created_at) VALUES('orphan-ci','https://example.com/orphan',NOW()-INTERVAL '10 minutes');" >/dev/null
docker compose exec -T app node --input-type=module -e "import { reconcileCreateIntents } from './src/consistency.mjs'; import { pool } from './src/db.mjs'; await reconcileCreateIntents({orphanAfterMs:0,batch:100}); await pool.end();"
orphan_status=$(curl -sS -o /dev/null -w '%{http_code}' http://localhost:8080/rest/v3/short-urls/orphan-ci -H "X-Api-Key: ${SHLINK_API_KEY}")
[[ "$orphan_status" == "404" ]]
intent_count=$(docker compose exec -T db psql -U postgres -d qh8z -Atc "SELECT COUNT(*) FROM shlink_create_intents WHERE short_code='orphan-ci';")
[[ "$intent_count" == "0" ]]

# Create durable sentinel data in each database through the surfaces they own.
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 -c "INSERT INTO audit_events(event_type,target_id) VALUES('backup.restore.sentinel','ci-restore');" >/dev/null
curl -fsS -X POST http://localhost:8080/rest/v3/short-urls \
  -H "X-Api-Key: ${SHLINK_API_KEY}" \
  -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/backup-restored","customSlug":"backup-ci"}' >/tmp/qh8z-backup-link.json

backup_path=$(bash scripts/backup.sh /tmp/qh8z-backups)
[[ -s "$backup_path" ]]
[[ -s "$backup_path.sha256" ]]

# backup.sh stops and restarts the app. The stop must drain through the Node
# lifecycle handler rather than falling back to Docker SIGKILL/abrupt exit.
docker compose --profile production logs --no-color app | grep -q '"event":"app.shutdown_completed"'

# A successful backup must return the previously running public stack without
# recreating healthy dependencies or leaving the edge unavailable.
for _ in $(seq 1 60); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1 && curl -kfsS https://localhost/healthz >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS http://localhost:3000/readyz >/dev/null
curl -kfsS https://localhost/healthz >/dev/null
assert_db_isolation

# Destroy both sentinels after the snapshot so only restore can bring them back.
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 -c "DELETE FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';" >/dev/null
curl -fsS -X DELETE http://localhost:8080/rest/v3/short-urls/backup-ci -H "X-Api-Key: ${SHLINK_API_KEY}" >/dev/null
count=$(docker compose exec -T db psql -U postgres -d qh8z -Atc "SELECT COUNT(*) FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';")
[[ "$count" == "0" ]]

CONFIRM_RESTORE=YES bash scripts/restore.sh "$backup_path"
ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1 && curl -kfsS https://localhost/healthz >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]]
assert_db_isolation

count=$(docker compose exec -T db psql -U postgres -d qh8z -Atc "SELECT COUNT(*) FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';")
[[ "$count" == "1" ]]
restored_redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/backup-ci)
[[ "$restored_redirect" == "https://example.com/backup-restored" ]]

echo 'QH8Z backup/restore drill passed with orphan cleanup, graceful app shutdown, database role isolation, and HTTPS recovery.'
