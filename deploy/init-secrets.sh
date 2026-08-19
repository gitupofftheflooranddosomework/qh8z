#!/bin/sh
set -eu

secret_dir="${QH8Z_SECRETS_DIR:-/etc/qh8z/secrets}"
secret_gid="${QH8Z_SECRET_GID:-65532}"

if [ "$(id -u)" -ne 0 ]; then
  echo "run this script as root so secrets can be created under $secret_dir" >&2
  exit 1
fi

install -d -m 0750 "$secret_dir"
chown "0:$secret_gid" "$secret_dir"
umask 027

secure_secret() {
  path="$1"
  chown "0:$secret_gid" "$path"
  chmod 0640 "$path"
}

generate_if_missing() {
  name="$1"
  bytes="$2"
  path="$secret_dir/$name"
  if [ -s "$path" ]; then
    secure_secret "$path"
    echo "keeping existing $path"
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr -d '\n' > "$path"
  else
    head -c "$bytes" /dev/urandom | base64 | tr -d '\n' > "$path"
  fi
  secure_secret "$path"
  echo "generated $path"
}

generate_if_missing postgres_password 48
generate_if_missing admin_token 48
generate_if_missing rate_limit_salt 48
generate_if_missing restic_password 48

for path in "$secret_dir"/*; do
  [ -f "$path" ] || continue
  secure_secret "$path"
done

cat <<EOF

Generated qh8z-owned secrets. Before deployment, create these service-provided secret files, then rerun this script so they are secured as root:$secret_gid with mode 0640:
  $secret_dir/smtp_password
  $secret_dir/webrisk_api_key
  $secret_dir/stripe_secret_key
  $secret_dir/stripe_webhook_secret
  $secret_dir/restic_s3_access_key
  $secret_dir/restic_s3_secret_key
  $secret_dir/alertmanager_discord_webhook

Do not commit those files to Git.
EOF
