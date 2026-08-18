#!/usr/bin/env bash
set -euo pipefail
umask 077

out_dir="${1:-./backups}"
env_file="${2:-}"
mkdir -p "$out_dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
out="$out_dir/qh8z-backup-$stamp.tar.gz"
tmp=$(mktemp -d)
app_was_running=0
shlink_was_running=0
caddy_was_running=0
backup_complete=0
compose=(docker compose)
[[ -n "$env_file" ]] && compose+=(--env-file "$env_file")

restart_previous_services() {
  # When the public edge was running, start the edge and all dependencies in a
  # single no-recreate transaction. Separate `up` calls can unexpectedly
  # recreate an already healthy dependency.
  if [[ "$caddy_was_running" == "1" ]]; then
    "${compose[@]}" --profile production up -d --no-recreate caddy >/dev/null 2>&1 || true
    return
  fi
  local services=()
  [[ "$shlink_was_running" == "1" ]] && services+=(shlink)
  [[ "$app_was_running" == "1" ]] && services+=(app)
  if [[ ${#services[@]} -gt 0 ]]; then
    "${compose[@]}" up -d --no-recreate "${services[@]}" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  rm -rf "$tmp"
  restart_previous_services
  if [[ "$backup_complete" != "1" ]]; then echo "Backup failed; previously running services were restarted where possible." >&2; fi
}
trap cleanup EXIT

[[ -n "$("${compose[@]}" ps --status running -q app 2>/dev/null || true)" ]] && app_was_running=1
[[ -n "$("${compose[@]}" ps --status running -q shlink 2>/dev/null || true)" ]] && shlink_was_running=1
[[ -n "$("${compose[@]}" --profile production ps --status running -q caddy 2>/dev/null || true)" ]] && caddy_was_running=1

# Quiesce every service that can create application or visit state. This creates
# a brief maintenance window but avoids restoring mismatched QH8Z/Shlink state.
[[ "$caddy_was_running" == "1" ]] && "${compose[@]}" --profile production stop caddy >/dev/null
[[ "$app_was_running" == "1" ]] && "${compose[@]}" stop app >/dev/null
[[ "$shlink_was_running" == "1" ]] && "${compose[@]}" stop shlink >/dev/null

# QH8Z owns link/account state while Shlink owns redirect/visit state. Dump both
# in PostgreSQL custom format so restores can recreate clean databases.
"${compose[@]}" exec -T db pg_dump -U qh8z -d qh8z --format=custom --no-owner --no-acl > "$tmp/qh8z.dump"
"${compose[@]}" exec -T db pg_dump -U qh8z -d shlink --format=custom --no-owner --no-acl > "$tmp/shlink.dump"
test -s "$tmp/qh8z.dump"
test -s "$tmp/shlink.dump"
printf 'created_at=%s\nformat=postgres-custom-v1\ncontains=qh8z,shlink\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp/MANIFEST.txt"
tar -C "$tmp" -czf "$out" MANIFEST.txt qh8z.dump shlink.dump
(
  cd "$out_dir"
  sha256sum "$(basename "$out")" > "$(basename "$out").sha256"
)
backup_complete=1
echo "$out"
