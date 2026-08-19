#!/bin/sh
set -eu

if ! command -v git >/dev/null 2>&1; then
  echo "secret scan requires git" >&2
  exit 2
fi

if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  echo "secret scan requires a full Git history checkout" >&2
  exit 2
fi

failed=0

scan_rule() {
  label="$1"
  regex="$2"
  for commit in $(git rev-list --all); do
    if git grep -I -q -E "$regex" "$commit" -- . 2>/dev/null; then
      echo "secret scan failed: rule=$label commit=$commit" >&2
      failed=1
      return
    fi
  done
}

scan_direct_secret_assignments() {
  regex='(STRIPE_SECRET_KEY|STRIPE_WEBHOOK_SECRET|WEBRISK_API_KEY|SMTP_PASSWORD|DATABASE_PASSWORD|QH8Z_ADMIN_TOKEN|QH8Z_RATE_LIMIT_SALT|AWS_SECRET_ACCESS_KEY|RESTIC_PASSWORD)=[A-Za-z0-9+/=_:.-]{16,}'

  for commit in $(git rev-list --all); do
    matches="$(git grep -I -h -E "$regex" "$commit" -- . 2>/dev/null || true)"
    [ -n "$matches" ] || continue

    if ! printf '%s\n' "$matches" | while IFS= read -r line; do
      value="${line#*=}"
      case "$value" in
        change-me-*)
          # Historical .env.example files deliberately use this documented
          # non-secret prefix. Keep these fixtures scannable without treating
          # them as leaked opaque credentials.
          continue
          ;;
        *)
          exit 1
          ;;
      esac
    done; then
      echo "secret scan failed: rule=direct_secret_assignment commit=$commit" >&2
      failed=1
      return
    fi
  done
}

# Provider formats are intentionally length-bounded so short test fixtures such
# as sk_test_qh8z and whsec_test are not mistaken for real credentials.
scan_rule stripe_secret 'sk_(live|test)_[A-Za-z0-9]{20,}'
scan_rule stripe_restricted 'rk_(live|test)_[A-Za-z0-9]{20,}'
scan_rule stripe_webhook 'whsec_[A-Za-z0-9]{20,}'
scan_rule google_api_key 'AIza[0-9A-Za-z_-]{35}'
scan_rule aws_access_key '(AKIA|ASIA)[0-9A-Z]{16}'
scan_rule github_token '(gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{50,})'
scan_rule sendgrid_key 'SG\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{40,}'
scan_rule discord_webhook 'https://(discord(app)?\.com)/api/webhooks/[0-9]{8,}/[A-Za-z0-9_-]{20,}'
scan_rule private_key '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----'

# qh8z production secrets may also be opaque random strings with no provider
# prefix. Detect direct tracked assignments while allowing the explicit
# change-me-* placeholders used by historical example configuration.
scan_direct_secret_assignments

if [ "$failed" -ne 0 ]; then
  echo "secret scan failed; inspect the named rule and commit locally without printing credentials into CI logs" >&2
  exit 1
fi

echo "full Git history secret scan passed"
