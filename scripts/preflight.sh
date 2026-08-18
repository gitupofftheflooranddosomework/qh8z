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
require_int_range() {
  local key="$1" min="$2" max="$3" v n
  v=$(value "$key")
  if [[ ! "$v" =~ ^[0-9]+$ ]]; then
    echo "FAIL: $key must be an integer between $min and $max" >&2
    fail=1
    return
  fi
  n=$((10#$v))
  if (( n < min || n > max )); then
    echo "FAIL: $key must be an integer between $min and $max" >&2
    fail=1
  fi
}
valid_email() {
  local v="$1"
  [[ "$v" =~ ^[^[:space:]\<\>@]+@[^[:space:]\<\>@]+\.[^[:space:]\<\>@]+$ ]]
}

[[ "$(value NODE_ENV)" == "production" ]] || { echo "FAIL: NODE_ENV must be production" >&2; fail=1; }
[[ "$(value PUBLIC_LAUNCH_MODE)" == "true" ]] || { echo "FAIL: PUBLIC_LAUNCH_MODE must be true" >&2; fail=1; }
require_true COOKIE_SECURE
require_true EMAIL_VERIFICATION_REQUIRED
require_true WEB_RISK_REQUIRED
require_true TURNSTILE_REQUIRED
for key in QH8Z_DOMAIN POSTGRES_PASSWORD QH8Z_DB_PASSWORD SHLINK_DB_PASSWORD SHLINK_API_KEY ADMIN_EMAIL MFA_ENCRYPTION_KEY SUPPORT_EMAIL ABUSE_EMAIL WEB_RISK_API_KEY TURNSTILE_SITE_KEY TURNSTILE_SECRET_KEY TERMS_VERSION SMTP_HOST MAIL_FROM LEGAL_OPERATOR_NAME LEGAL_JURISDICTION; do require_nonplaceholder "$key"; done

domain=$(value QH8Z_DOMAIN)
if [[ ! "$domain" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ || "$domain" == *..* || "$domain" == .* || "$domain" == *. ]]; then
  echo "FAIL: QH8Z_DOMAIN must be a plain hostname" >&2
  fail=1
fi
expected_origin="https://${domain}"
[[ "$(value APP_BASE_URL)" == "$expected_origin" ]] || { echo "FAIL: APP_BASE_URL must exactly equal $expected_origin" >&2; fail=1; }
[[ "$(value PUBLIC_SHORT_BASE_URL)" == "$expected_origin" ]] || { echo "FAIL: PUBLIC_SHORT_BASE_URL must exactly equal $expected_origin" >&2; fail=1; }

admin_email=$(value ADMIN_EMAIL)
support_email=$(value SUPPORT_EMAIL)
abuse_email=$(value ABUSE_EMAIL)
if ! valid_email "$admin_email" || [[ "$admin_email" == *.example || "$admin_email" == *replace-with* ]]; then echo "FAIL: ADMIN_EMAIL must be a real administrator address" >&2; fail=1; fi
valid_email "$support_email" || { echo "FAIL: SUPPORT_EMAIL must be a valid address" >&2; fail=1; }
valid_email "$abuse_email" || { echo "FAIL: ABUSE_EMAIL must be a valid address" >&2; fail=1; }

legal_operator=$(value LEGAL_OPERATOR_NAME)
legal_jurisdiction=$(value LEGAL_JURISDICTION)
if [[ "$legal_operator" == *'<'* || "$legal_operator" == *'>'* || "$legal_jurisdiction" == *'<'* || "$legal_jurisdiction" == *'>'* ]]; then
  echo "FAIL: legal operator and jurisdiction fields must be plain text without HTML markup" >&2
  fail=1
fi

[[ "$(value MFA_ENCRYPTION_KEY)" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "FAIL: MFA_ENCRYPTION_KEY must be exactly 64 hex characters" >&2; fail=1; }
for key in POSTGRES_PASSWORD QH8Z_DB_PASSWORD SHLINK_DB_PASSWORD SHLINK_API_KEY; do
  [[ "$(value "$key")" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "FAIL: $key must be 32 random bytes encoded as 64 hex characters" >&2; fail=1; }
done
bootstrap=$(value ADMIN_BOOTSTRAP_SECRET)
if [[ -n "$bootstrap" && ( "$bootstrap" == *replace-with* || "$bootstrap" == *change-me* || ${#bootstrap} -lt 24 ) ]]; then
  echo "FAIL: ADMIN_BOOTSTRAP_SECRET must be blank after bootstrap or a strong one-time secret" >&2
  fail=1
fi
[[ "$(value MAIL_MODE)" == "smtp" ]] || { echo "FAIL: MAIL_MODE must be smtp" >&2; fail=1; }
require_int_range SMTP_PORT 1 65535
require_int_range SESSION_TTL_DAYS 1 90
require_int_range ADMIN_SESSION_HOURS 1 24
require_int_range DATA_RETENTION_DAYS 30 3650
require_int_range REPUTATION_RECHECK_HOURS 1 168
require_int_range REPUTATION_RECHECK_BATCH 1 1000
require_int_range REPUTATION_WORKER_MINUTES 1 60

if [[ $fail -ne 0 ]]; then exit 1; fi

echo "Validating Compose configuration..."
docker compose --env-file "$ENV_FILE" --profile production config >/dev/null

echo "Validating Caddy configuration..."
QH8Z_DOMAIN="$domain"
docker run --rm -e "QH8Z_DOMAIN=${QH8Z_DOMAIN}" -v "$PWD/infra/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null

echo "QH8Z production preflight passed without printing secrets."
