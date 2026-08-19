#!/bin/sh
set -eu

if [ "${QH8Z_FAILURE_TEST_ACK:-}" != "maintenance-window" ]; then
  echo "refusing disruptive test; set QH8Z_FAILURE_TEST_ACK=maintenance-window" >&2
  exit 2
fi

compose_file="${QH8Z_COMPOSE_FILE:-deploy/compose.production.yml}"
env_file="${QH8Z_ENV_FILE:-deploy/.env.production}"

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

wait_ready() {
  attempts=0
  while [ "$attempts" -lt 30 ]; do
    if compose exec -T qh8z /healthcheck >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 2
  done
  echo "qh8z did not become ready" >&2
  return 1
}

cleanup() {
  compose start postgres >/dev/null 2>&1 || true
  compose start qh8z >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

wait_ready

echo "stopping PostgreSQL to verify readiness failure"
compose stop postgres
sleep 3
if compose exec -T qh8z /healthcheck >/dev/null 2>&1; then
  echo "readiness probe unexpectedly passed while PostgreSQL was stopped" >&2
  exit 1
fi

echo "restarting PostgreSQL and waiting for recovery"
compose start postgres
wait_ready

echo "restarting qh8z to verify application recovery"
compose restart qh8z
wait_ready

echo "failure test passed"
trap - EXIT INT TERM
