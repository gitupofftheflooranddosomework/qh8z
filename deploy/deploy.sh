#!/bin/sh
set -eu

env_file="${QH8Z_ENV_FILE:-deploy/.env.production}"
compose_file="${QH8Z_COMPOSE_FILE:-deploy/compose.production.yml}"

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

if [ ! -f "$env_file" ]; then
  echo "missing production environment file: $env_file" >&2
  exit 2
fi

if [ "${QH8Z_ALLOW_DIRTY_DEPLOY:-0}" != "1" ]; then
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "refusing deployment from a dirty Git working tree" >&2
    exit 2
  fi
fi

sha="$(git rev-parse HEAD)"
echo "deploying qh8z commit $sha"

compose config --quiet

if [ -n "$(compose ps -q postgres 2>/dev/null || true)" ] && [ "${QH8Z_SKIP_PREDEPLOY_BACKUP:-0}" != "1" ]; then
  echo "taking pre-deploy encrypted database backup"
  compose run --rm -e BACKUP_ONCE=1 backup
else
  echo "no running production database found; skipping pre-deploy backup"
fi

compose pull prometheus alertmanager
compose build --pull qh8z caddy postgres backup
compose up -d --remove-orphans

attempts=0
while ! compose exec -T qh8z /healthcheck >/dev/null 2>&1; do
  attempts=$((attempts + 1))
  if [ "$attempts" -ge 45 ]; then
    echo "deployment failed readiness check" >&2
    compose ps >&2 || true
    compose logs --since 10m qh8z postgres caddy prometheus alertmanager >&2 || true
    exit 1
  fi
  sleep 2
done

echo "qh8z commit $sha is ready"
