#!/usr/bin/env bash
set -euo pipefail

TARGET_REF="${1:-origin/main}"
ENV_FILE="${2:-.env}"
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

[[ -f "$ENV_FILE" ]] || { echo "Missing production env file: $ENV_FILE" >&2; exit 2; }
[[ -z "$(git status --porcelain --untracked-files=no)" ]] || { echo "Refusing deploy with tracked working-tree changes" >&2; exit 2; }

old_head=$(git rev-parse HEAD)
git fetch --tags --prune origin
target_head=$(git rev-parse --verify "${TARGET_REF}^{commit}")
backup_path=$(bash scripts/backup.sh "${QH8Z_BACKUP_DIR:-./backups}")

deploy_ok=0
rollback_code() {
  if [[ "$deploy_ok" != "1" ]]; then
    echo "Deployment failed; rolling application code back to $old_head" >&2
    git checkout --detach "$old_head" >/dev/null 2>&1 || true
    docker compose --env-file "$ENV_FILE" --profile production up -d --build --remove-orphans >/dev/null 2>&1 || true
    echo "Database backup retained at: $backup_path" >&2
  fi
}
trap rollback_code EXIT

git checkout --detach "$target_head" >/dev/null
bash scripts/preflight.sh "$ENV_FILE"
docker compose --env-file "$ENV_FILE" --profile production pull
docker compose --env-file "$ENV_FILE" --profile production up -d --build --remove-orphans

app_port=$(grep -E '^APP_DEV_PORT=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d "'\"" || true)
app_port=${app_port:-3000}
ready=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${app_port}/readyz" >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]] || { echo "QH8Z did not become ready after deploy" >&2; exit 1; }

public_url=$(grep -E '^APP_BASE_URL=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d "'\"" || true)
public_url=${public_url:-https://qh8z.com}
bash scripts/postdeploy.sh "$public_url"

deploy_ok=1
trap - EXIT
echo "QH8Z deployed: $target_head"
echo "Pre-deploy backup: $backup_path"
