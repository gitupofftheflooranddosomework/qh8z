#!/bin/sh
set -eu

secret_dir="${QH8Z_SECRETS_DIR:-/etc/qh8z/secrets}"

if [ "$(id -u)" -ne 0 ]; then
  echo "run this script as root so secrets can be created under $secret_dir" >&2
  exit 1
fi

install -d -m 0700 "$secret_dir"
umask 077

generate_if_missing() {
  name="$1"
  bytes="$2"
  path="$secret_dir/$name"
  if [ -s "$path" ]; then
    echo "keeping existing $path"
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr -d '\n' > "$path"
  else
    head -c "$bytes" /dev/urandom | base64 | tr -d '\n' > "$path"
  fi
  chmod 0600 "$path"
  echo "generated $path"
}

generate_if_missing postgres_password 48
generate_if_missing admin_token 48
generate_if_missing rate_limit_salt 48
generate_if_missing restic_password 48

cat <<EOF

Generated qh8z-owned secrets. Before deployment, create these service-provided secret files with mode 0600:
  $secret_dir/smtp_password
  $secret_dir/webrisk_api_key
  $secret_dir/stripe_secret_key
  $secret_dir/stripe_webhook_secret
  $secret_dir/restic_s3_access_key
  $secret_dir/restic_s3_secret_key
  $secret_dir/alertmanager_discord_webhook

Do not commit those files to Git.
EOF
