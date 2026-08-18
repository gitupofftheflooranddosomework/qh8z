#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then echo "usage: $0 <qh8z-backup-*.tar.gz> [env-file]" >&2; exit 2; fi
backup="$1"
env_file="${2:-}"
[[ -f "$backup" ]] || { echo "Backup not found: $backup" >&2; exit 2; }
[[ "${CONFIRM_RESTORE:-}" == "YES" ]] || { echo "This replaces both QH8Z and Shlink databases. Set CONFIRM_RESTORE=YES to continue." >&2; exit 3; }

tmp=$(mktemp -d)
app_was_running=0
shlink_was_running=0
caddy_was_running=0
compose=(docker compose)
[[ -n "$env_file" ]] && compose+=(--env-file "$env_file")
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

checksum="$backup.sha256"
if [[ -f "$checksum" ]]; then
  (
    cd "$(dirname "$backup")"
    sha256sum -c "$(basename "$checksum")"
  )
else
  echo "WARNING: no checksum file found at $checksum" >&2
fi

entries=$(tar -tzf "$backup")
if printf '%s\n' "$entries" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
  echo "Refusing archive with unsafe paths" >&2
  exit 4
fi
for required in MANIFEST.txt qh8z.dump shlink.dump; do
  printf '%s\n' "$entries" | grep -Fxq "$required" || { echo "Backup is missing $required" >&2; exit 4; }
done
tar -xzf "$backup" -C "$tmp" MANIFEST.txt qh8z.dump shlink.dump

test -s "$tmp/qh8z.dump"
test -s "$tmp/shlink.dump"

[[ -n "$("${compose[@]}" ps --status running -q app 2>/dev/null || true)" ]] && app_was_running=1
[[ -n "$("${compose[@]}" ps --status running -q shlink 2>/dev/null || true)" ]] && shlink_was_running=1
[[ -n "$("${compose[@]}" --profile production ps --status running -q caddy 2>/dev/null || true)" ]] && caddy_was_running=1

# Stop writers/readers before recreating both application databases. If restore
# fails, services deliberately remain stopped rather than serving partial data.
[[ "$caddy_was_running" == "1" ]] && "${compose[@]}" --profile production stop caddy >/dev/null
[[ "$app_was_running" == "1" ]] && "${compose[@]}" stop app >/dev/null
[[ "$shlink_was_running" == "1" ]] && "${compose[@]}" stop shlink >/dev/null

"${compose[@]}" exec -T db dropdb -U qh8z --if-exists qh8z
"${compose[@]}" exec -T db createdb -U qh8z -O qh8z qh8z
cat "$tmp/qh8z.dump" | "${compose[@]}" exec -T db pg_restore -U qh8z -d qh8z --no-owner --no-acl --exit-on-error

"${compose[@]}" exec -T db dropdb -U qh8z --if-exists shlink
"${compose[@]}" exec -T db createdb -U qh8z -O qh8z shlink
cat "$tmp/shlink.dump" | "${compose[@]}" exec -T db pg_restore -U qh8z -d shlink --no-owner --no-acl --exit-on-error

# Restart the exact service set that was running before the restore without
# allowing Compose to recreate healthy dependencies.
if [[ "$caddy_was_running" == "1" ]]; then
  "${compose[@]}" --profile production up -d --no-recreate caddy >/dev/null
else
  services=()
  [[ "$shlink_was_running" == "1" ]] && services+=(shlink)
  [[ "$app_was_running" == "1" ]] && services+=(app)
  if [[ ${#services[@]} -gt 0 ]]; then "${compose[@]}" up -d --no-recreate "${services[@]}" >/dev/null; fi
fi

echo "Restore completed. Verify /readyz and application behavior before reopening traffic."
