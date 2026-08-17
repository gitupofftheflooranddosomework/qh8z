#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env}"
[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE. Start from .env.production.example." >&2; exit 2; }
command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }
docker compose version >/dev/null

value() {
  local key="$1"
  local line
  line=$(grep -E "^${key}=" "$ENV_FILE" | tail -n 1 || true)
  printf '%s' "${line#*=}" | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//"
}

fail=0
require_nonplaceholder() {
  local key="$1" v
  v=$(value "$key")
  if [[ -z "$v" || "$v" == *replace-with* || "$v" == *change-me* ]]; then
    echo "FAIL: $key is missing or still a placeholder" >&2
    fail=1
  fi
}
require_true() {
  local key="$1" v
  v=$(value "$key" | tr '[:upper:]' '[:lower:]')
  if [[ "$v" != "true" && "$v" != "1" && "$v" != "yes" && "$v" != "on" ]]; then
    echo "FAIL: $key must be true" >&2
    fail=1
  fi
}

[[ "$(value NODE_ENV)" == "production" ]] || { echo "FAIL: NODE_ENV must be production" >&2; fail=1; }
[[ "$(value PUBLIC_LAUNCH_MODE)" == "true" ]] || { echo "FAIL: PUBLIC_LAUNCH_MODE must be true" >&2; fail=1; }
[[ "$(value APP_BASE_URL)" == https://* ]] || { echo "FAIL: APP_BASE_URL must use https://" >&2; fail=1; }
[[ "$(value PUBLIC_SHORT_BASE_URL)" == https://* ]] || { echo "FAIL: PUBLIC_SHORT_BASE_URL must use https://" >&2; fail=1; }
require_true COOKIE_SECURE
require_true EMAIL_VERIFICATION_REQUIRED
require_true WEB_RISK_REQUIRED
require_true TURNSTILE_REQUIRED
for key in POSTGRES_PASSWORD SHLINK_API_KEY ADMIN_EMAIL ADMIN_BOOTSTRAP_SECRET MFA_ENCRYPTION_KEY SUPPORT_EMAIL ABUSE_EMAIL WEB_RISK_API_KEY TURNSTILE_SITE_KEY TURNSTILE_SECRET_KEY TERMS_VERSION SMTP_HOST MAIL_FROM LEGAL_OPERATOR_NAME LEGAL_JURISDICTION; do require_nonplaceholder "$key"; done
[[ "$(value ADMIN_EMAIL)" != *@example.com ]] || { echo "FAIL: ADMIN_EMAIL must not use example.com" >&2; fail=1; }
[[ "$(value MFA_ENCRYPTION_KEY)" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "FAIL: MFA_ENCRYPTION_KEY must be exactly 64 hex characters" >&2; fail=1; }
for key in POSTGRES_PASSWORD SHLINK_API_KEY ADMIN_BOOTSTRAP_SECRET; do v=$(value "$key"); [[ ${#v} -ge 24 ]] || { echo "FAIL: $key must be at least 24 characters" >&2; fail=1; }; done
[[ "$(value MAIL_MODE)" == "smtp" ]] || { echo "FAIL: MAIL_MODE must be smtp" >&2; fail=1; }

if [[ $fail -ne 0 ]]; then exit 1; fi

echo "Validating Compose configuration..."
docker compose --env-file "$ENV_FILE" --profile production config >/dev/null

echo "Validating Caddy configuration..."
QH8Z_DOMAIN=$(value QH8Z_DOMAIN)
docker run --rm -e "QH8Z_DOMAIN=${QH8Z_DOMAIN}" -v "$PWD/infra/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null

echo "QH8Z production preflight passed without printing secrets."
