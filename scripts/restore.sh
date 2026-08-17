#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 1 ]]; then echo "usage: $0 <qh8z-postgres.sql.gz>" >&2; exit 2; fi
backup="$1"
test -f "$backup"
echo "This replaces data in the running PostgreSQL instance. Set CONFIRM_RESTORE=YES to continue." >&2
[[ "${CONFIRM_RESTORE:-}" == "YES" ]] || exit 3
gzip -dc "$backup" | docker compose exec -T db psql -U qh8z -d postgres
