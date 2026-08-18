#!/usr/bin/env bash
set -euo pipefail

: "${QH8Z_DB_PASSWORD:?QH8Z_DB_PASSWORD is required}"
: "${SHLINK_DB_PASSWORD:?SHLINK_DB_PASSWORD is required}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=qh8z_db_password="$QH8Z_DB_PASSWORD" \
  --set=shlink_db_password="$SHLINK_DB_PASSWORD" <<'SQL'
CREATE ROLE qh8z_app LOGIN PASSWORD :'qh8z_db_password';
CREATE ROLE shlink_app LOGIN PASSWORD :'shlink_db_password';

ALTER DATABASE qh8z OWNER TO qh8z_app;
REVOKE CONNECT ON DATABASE qh8z FROM PUBLIC;
GRANT CONNECT ON DATABASE qh8z TO qh8z_app;

CREATE DATABASE shlink OWNER shlink_app;
REVOKE CONNECT ON DATABASE shlink FROM PUBLIC;
GRANT CONNECT ON DATABASE shlink TO shlink_app;
SQL
