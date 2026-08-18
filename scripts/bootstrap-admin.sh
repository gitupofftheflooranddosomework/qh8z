#!/usr/bin/env bash
set -euo pipefail
ENV_FILE="${1:-.env}"
[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE" >&2; exit 2; }
read -r -s -p 'New QH8Z admin password: ' password; echo
read -r -s -p 'Confirm password: ' confirm; echo
[[ "$password" == "$confirm" ]] || { echo 'Passwords do not match' >&2; exit 2; }
bootstrap_secret=$(grep -E '^ADMIN_BOOTSTRAP_SECRET=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//')
[[ -n "$bootstrap_secret" ]] || { echo 'ADMIN_BOOTSTRAP_SECRET is missing' >&2; exit 2; }
docker compose --env-file "$ENV_FILE" exec -T \
  -e "BOOTSTRAP_ADMIN_PASSWORD=$password" \
  -e "BOOTSTRAP_ADMIN_SECRET=$bootstrap_secret" \
  app node src/bootstrap-admin.mjs
unset password confirm bootstrap_secret
