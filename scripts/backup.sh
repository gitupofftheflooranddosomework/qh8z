#!/usr/bin/env bash
set -euo pipefail
umask 077

out_dir="${1:-./backups}"
mkdir -p "$out_dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
out="$out_dir/qh8z-backup-$stamp.tar.gz"
tmp=$(mktemp -d)
app_was_running=0

cleanup() {
  rm -rf "$tmp"
  if [[ "$app_was_running" == "1" ]]; then docker compose up -d app >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

if [[ -n "$(docker compose ps -q app 2>/dev/null || true)" ]]; then
  app_was_running=1
  docker compose stop app >/dev/null
fi

# QH8Z owns link/account state while Shlink owns redirect/visit state. Dump both
# in PostgreSQL custom format so restores can recreate clean databases.
docker compose exec -T db pg_dump -U qh8z -d qh8z --format=custom --no-owner --no-acl > "$tmp/qh8z.dump"
docker compose exec -T db pg_dump -U qh8z -d shlink --format=custom --no-owner --no-acl > "$tmp/shlink.dump"
test -s "$tmp/qh8z.dump"
test -s "$tmp/shlink.dump"
printf 'created_at=%s\nformat=postgres-custom-v1\ncontains=qh8z,shlink\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp/MANIFEST.txt"
tar -C "$tmp" -czf "$out" MANIFEST.txt qh8z.dump shlink.dump
(
  cd "$out_dir"
  sha256sum "$(basename "$out")" > "$(basename "$out").sha256"
)
echo "$out"
