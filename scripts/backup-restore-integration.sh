#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  docker compose --profile production logs --no-color > /tmp/qh8z-backup-restore-compose.log 2>&1 || true
  docker compose --profile production down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker compose up -d --build db shlink app
ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]]

# Create durable sentinel data in each database through the surfaces they own.
docker compose exec -T db psql -U qh8z -d qh8z -v ON_ERROR_STOP=1 -c "INSERT INTO audit_events(event_type,target_id) VALUES('backup.restore.sentinel','ci-restore');" >/dev/null
curl -fsS -X POST http://localhost:8080/rest/v3/short-urls \
  -H "X-Api-Key: ${SHLINK_API_KEY}" \
  -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/backup-restored","customSlug":"backup-ci"}' >/tmp/qh8z-backup-link.json

backup_path=$(bash scripts/backup.sh /tmp/qh8z-backups)
[[ -s "$backup_path" ]]
[[ -s "$backup_path.sha256" ]]

# Destroy both sentinels after the snapshot so only restore can bring them back.
docker compose exec -T db psql -U qh8z -d qh8z -v ON_ERROR_STOP=1 -c "DELETE FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';" >/dev/null
curl -fsS -X DELETE http://localhost:8080/rest/v3/short-urls/backup-ci -H "X-Api-Key: ${SHLINK_API_KEY}" >/dev/null
count=$(docker compose exec -T db psql -U qh8z -d qh8z -Atc "SELECT COUNT(*) FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';")
[[ "$count" == "0" ]]

CONFIRM_RESTORE=YES bash scripts/restore.sh "$backup_path"
ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]]

count=$(docker compose exec -T db psql -U qh8z -d qh8z -Atc "SELECT COUNT(*) FROM audit_events WHERE event_type='backup.restore.sentinel' AND target_id='ci-restore';")
[[ "$count" == "1" ]]
restored_redirect=$(curl -sS -H 'Host: localhost' -o /dev/null -w '%{redirect_url}' http://localhost:8080/backup-ci)
[[ "$restored_redirect" == "https://example.com/backup-restored" ]]

echo 'QH8Z backup/restore drill passed for both application databases.'
