#!/usr/bin/env bash
set -euo pipefail

ORIGIN=http://localhost:3000
ADMIN_JAR=/tmp/qh8z-admin-cookies.txt
USER_JAR=/tmp/qh8z-user-cookies.txt

cleanup() {
  docker compose logs --no-color > /tmp/qh8z-compose.log 2>&1 || true
  docker compose --profile production down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

totp() {
  python3 - "$1" <<'PY'
import base64, hashlib, hmac, struct, sys, time
secret=sys.argv[1]
pad='='*((8-len(secret)%8)%8)
key=base64.b32decode(secret+pad, casefold=True)
counter=int(time.time())//30
msg=struct.pack('>Q', counter)
digest=hmac.new(key,msg,hashlib.sha1).digest()
o=digest[-1]&15
code=(struct.unpack('>I',digest[o:o+4])[0]&0x7fffffff)%1000000
print(f'{code:06d}')
PY
}

docker compose --profile production config >/dev/null
docker run --rm -e QH8Z_DOMAIN=qh8z.com -v "$PWD/infra/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null

docker compose up -d --build db shlink app

ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/tmp/qh8z-ready.json 2>/dev/null; then ready=1; break; fi
  sleep 2
done
if [[ "$ready" != "1" ]]; then docker compose logs --no-color; exit 1; fi

# Start the real production edge on localhost TLS and verify upstream management is not exposed.
docker compose --profile production up -d caddy
edge_ready=0
for _ in $(seq 1 60); do
  if curl -kfsS https://localhost/healthz >/tmp/qh8z-edge-health.json 2>/dev/null; then edge_ready=1; break; fi
  sleep 1
done
if [[ "$edge_ready" != "1" ]]; then docker compose --profile production logs --no-color caddy; exit 1; fi
edge_api_status=$(curl -ksS -o /dev/null -w '%{http_code}' https://localhost/rest/health)
[[ "$edge_api_status" == "404" ]]
curl -kfsS -D /tmp/qh8z-edge-headers.txt -o /dev/null https://localhost/
grep -qi '^strict-transport-security:' /tmp/qh8z-edge-headers.txt
grep -qi '^x-content-type-options: nosniff' /tmp/qh8z-edge-headers.txt

# Bootstrap admin locally (never through a public HTTP bypass), log in, then enroll MFA.
docker compose exec -T -e BOOTSTRAP_ADMIN_PASSWORD=correct-horse-battery -e BOOTSTRAP_ADMIN_SECRET=qh8z-ci-admin-bootstrap-secret-2026 app node src/bootstrap-admin.mjs >/tmp/qh8z-admin-bootstrap.txt
curl -fsS -c "$ADMIN_JAR" -H 'content-type: application/json' -d '{"email":"admin@example.com","password":"correct-horse-battery"}' http://localhost:3000/api/auth/login >/tmp/qh8z-admin-first-login.json
mfa_setup=$(curl -fsS -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"password":"correct-horse-battery"}' http://localhost:3000/api/account/mfa/setup)
printf '%s' "$mfa_setup" >/tmp/qh8z-mfa-setup.json
mfa_secret=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["secret"])' </tmp/qh8z-mfa-setup.json)
mfa_code=$(totp "$mfa_secret")
mfa_confirm=$(curl -fsS -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d "{\"code\":\"$mfa_code\"}" http://localhost:3000/api/account/mfa/confirm)
printf '%s' "$mfa_confirm" >/tmp/qh8z-mfa-confirm.json
recovery_code=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["recoveryCodes"][0])' </tmp/qh8z-mfa-confirm.json)

curl -fsS -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -X POST http://localhost:3000/api/auth/logout >/dev/null
login_json=$(curl -fsS -H 'content-type: application/json' -d '{"email":"admin@example.com","password":"correct-horse-battery"}' http://localhost:3000/api/auth/login)
printf '%s' "$login_json" >/tmp/qh8z-admin-login.json
challenge=$(python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["mfaRequired"] is True; print(d["challengeToken"])' </tmp/qh8z-admin-login.json)
curl -fsS -c "$ADMIN_JAR" -H 'content-type: application/json' -d "{\"challengeToken\":\"$challenge\",\"code\":\"$recovery_code\"}" http://localhost:3000/api/auth/mfa >/tmp/qh8z-admin-mfa-login.json

# Normal users must verify email before they can create redirects.
register_json=$(curl -fsS -c "$USER_JAR" -H 'content-type: application/json' -d '{"name":"CI User","email":"user@example.com","password":"correct-horse-battery","acceptTerms":true}' http://localhost:3000/api/auth/register)
printf '%s' "$register_json" >/tmp/qh8z-user.json
verification_token=$(python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["verificationRequired"] is True; print(d["debugVerificationToken"])' </tmp/qh8z-user.json)
status=$(curl -sS -o /tmp/qh8z-unverified.json -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"https://example.com/nope"}' http://localhost:3000/api/links)
[[ "$status" == "403" ]]
curl -fsS -H 'content-type: application/json' -d "{\"token\":\"$verification_token\"}" http://localhost:3000/api/auth/verify-email >/dev/null

# Reserved aliases and local/private destinations are rejected.
status=$(curl -sS -o /tmp/qh8z-reserved.json -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"https://example.com","customSlug":"api"}' http://localhost:3000/api/links)
[[ "$status" == "400" ]]
status=$(curl -sS -o /tmp/qh8z-private.json -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"http://127.0.0.1/internal"}' http://localhost:3000/api/links)
[[ "$status" == "400" ]]

create_json=$(curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"https://example.com/one","customSlug":"ci-link","title":"CI link"}' http://localhost:3000/api/links)
printf '%s' "$create_json" >/tmp/qh8z-link.json
link_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["link"]["id"])' </tmp/qh8z-link.json)
redirect_1=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/ci-link)
[[ "$redirect_1" == "https://example.com/one" ]]

curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X PATCH -H 'content-type: application/json' -d '{"longUrl":"https://example.com/two","title":"Updated CI link"}' "http://localhost:3000/api/links/${link_id}" >/tmp/qh8z-edited.json
redirect_2=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/ci-link)
[[ "$redirect_2" == "https://example.com/two" ]]
curl -fsS -b "$USER_JAR" "http://localhost:3000/api/links/${link_id}/stats" >/tmp/qh8z-stats.json
curl -fsS -b "$USER_JAR" "http://localhost:3000/api/links/${link_id}/qr.svg" | grep -q '<svg'

# Public abuse intake and admin moderation.
curl -fsS -H 'content-type: application/json' -d '{"shortCode":"ci-link","email":"reporter@example.com","reason":"Integration test report","category":"phishing"}' http://localhost:3000/api/report >/tmp/qh8z-report.json
curl -fsS -b "$ADMIN_JAR" http://localhost:3000/api/admin/reports >/tmp/qh8z-reports.json
grep -q 'Integration test report' /tmp/qh8z-reports.json
curl -fsS -b "$ADMIN_JAR" http://localhost:3000/api/admin/users >/tmp/qh8z-users.json
grep -q 'user@example.com' /tmp/qh8z-users.json

# Password recovery is generic externally but exposes a token only in CI/dev mode.
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST http://localhost:3000/api/auth/logout >/dev/null
forgot=$(curl -fsS -H 'content-type: application/json' -d '{"email":"user@example.com"}' http://localhost:3000/api/auth/forgot-password)
printf '%s' "$forgot" >/tmp/qh8z-forgot.json
reset_token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["debugResetToken"])' </tmp/qh8z-forgot.json)
curl -fsS -H 'content-type: application/json' -d "{\"token\":\"$reset_token\",\"password\":\"correct-horse-battery-new\"}" http://localhost:3000/api/auth/reset-password >/dev/null
curl -fsS -c "$USER_JAR" -H 'content-type: application/json' -d '{"email":"user@example.com","password":"correct-horse-battery-new"}' http://localhost:3000/api/auth/login >/tmp/qh8z-user-login.json

# Suspension revokes sessions and disables all active redirects.
second=$(curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"https://example.com/three","customSlug":"ci-two"}' http://localhost:3000/api/links)
printf '%s' "$second" >/tmp/qh8z-second.json
user_id=$(python3 -c 'import json; d=json.load(open("/tmp/qh8z-users.json")); print(next(x["id"] for x in d["users"] if x["email"]=="user@example.com"))')
curl -fsS -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"reason":"CI suspension"}' "http://localhost:3000/api/admin/users/${user_id}/suspend" >/tmp/qh8z-suspend.json
status=$(curl -sS -o /dev/null -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -d '{"longUrl":"https://example.com/four"}' http://localhost:3000/api/links)
[[ "$status" == "401" ]]
for slug in ci-link ci-two; do disabled=$(curl -ksS -o /dev/null -w '%{redirect_url}' "https://localhost/$slug" || true); [[ "$disabled" != https://example.com/* ]]; done
curl -fsS -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/admin/users/${user_id}/unsuspend" >/dev/null

# Destructive deletion on an MFA-protected account requires second-factor proof.
status=$(curl -sS -o /tmp/qh8z-delete-admin.json -w '%{http_code}' -b "$ADMIN_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' -X DELETE -d '{"password":"correct-horse-battery"}' http://localhost:3000/api/account)
[[ "$status" == "401" ]]
grep -q 'invalid_mfa_code' /tmp/qh8z-delete-admin.json

echo 'QH8Z public-readiness integration smoke test passed.'
