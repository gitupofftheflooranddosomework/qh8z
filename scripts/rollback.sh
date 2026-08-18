#!/usr/bin/env bash
set -euo pipefail

[[ $# -ge 1 ]] || { echo "usage: $0 <commit-or-ref> [env-file]" >&2; exit 2; }
TARGET_REF="$1"
ENV_FILE="${2:-.env}"
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

[[ -f "$ENV_FILE" ]] || { echo "Missing production env file: $ENV_FILE" >&2; exit 2; }
[[ -z "$(git status --porcelain --untracked-files=no)" ]] || { echo "Refusing rollback with tracked working-tree changes" >&2; exit 2; }

git fetch --tags --prune origin
target_head=$(git rev-parse --verify "${TARGET_REF}^{commit}")
backup_path=$(bash scripts/backup.sh "${QH8Z_BACKUP_DIR:-./backups}" "$ENV_FILE")
git checkout --detach "$target_head" >/dev/null

docker compose --env-file "$ENV_FILE" --profile production up -d --build --remove-orphans
app_port=$(grep -E '^APP_DEV_PORT=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d "'\"" || true)
app_port=${app_port:-3000}
ready=0
for _ in $(seq 1 60); do
  if curl --connect-timeout 2 --max-time 5 -fsS "http://127.0.0.1:${app_port}/readyz" >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]] || { echo "Rolled-back code did not become ready. Services were left running for inspection; backup: $backup_path" >&2; exit 1; }

public_url=$(grep -E '^APP_BASE_URL=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d "'\"" || true)
public_url=${public_url:-https://qh8z.com}
bash scripts/postdeploy.sh "$public_url"

echo "QH8Z code rollback deployed: $target_head"
echo "Pre-rollback database backup: $backup_path"
echo "This command rolls application code back; it does not automatically rewind database contents."
