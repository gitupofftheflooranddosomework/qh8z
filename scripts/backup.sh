#!/usr/bin/env bash
set -euo pipefail
umask 077
out_dir="${1:-./backups}"
mkdir -p "$out_dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
out="$out_dir/qh8z-postgres-$stamp.sql.gz"
docker compose exec -T db pg_dumpall -U qh8z | gzip -9 > "$out"
test -s "$out"
sha256sum "$out" > "$out.sha256"
echo "$out"
