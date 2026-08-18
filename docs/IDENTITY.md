# Identity, workspaces, and API keys

qh8z uses workspace ownership for all authenticated link-management APIs. Public short-link redirects remain unauthenticated.

## Account lifecycle

1. `POST /api/v1/auth/register` creates a user, a default workspace with the user as owner, a 30-day session, and a 24-hour email-verification token.
2. Verification is available through `POST /api/v1/auth/verify-email` or the link sent by email.
3. `POST /api/v1/auth/login` creates a new session. Unverified users may sign in so they can request another verification email, but verified email is required for link creation and privileged workspace actions.
4. `POST /api/v1/auth/logout` deletes the current session.
5. `POST /api/v1/auth/resend-verification` creates and sends a new verification token.

Passwords are stored as salted PBKDF2-HMAC-SHA256 hashes. Session, verification, and API-key plaintext secrets are never stored; PostgreSQL stores only their SHA-256 hashes.

## Sessions

Browser sessions use an `HttpOnly` `SameSite=Lax` cookie named `qh8z_session`. Production cookies are also `Secure`. Session secrets can also be supplied as Bearer tokens for non-browser clients.

Cookie-authenticated unsafe requests reject an explicitly cross-site `Sec-Fetch-Site` value or an `Origin` that does not match `QH8Z_BASE_URL`.

## Workspaces

Use `X-QH8Z-Workspace` to select a workspace for routes that are not workspace-qualified. If it is omitted for a session, qh8z selects the user's oldest workspace. API keys are permanently scoped to one workspace and reject a conflicting workspace header.

Roles are:

- `owner`
- `admin`
- `member`

Owners and admins can add existing qh8z users to a workspace. API-key creation requires a verified owner/admin browser session.

## API keys

Create a key with:

```text
POST /api/v1/workspaces/{workspace}/api-keys
```

Allowed scopes are:

- `links:read`
- `links:write`
- `analytics:read`
- `workspace:admin`

The response contains the plaintext `qh8z_sk_...` secret once. Only its hash is stored. Send it as:

```text
Authorization: Bearer qh8z_sk_...
```

## Email delivery

Development defaults to `QH8Z_EMAIL_MODE=log`, which writes verification URLs to structured logs.

Production refuses to start unless `QH8Z_EMAIL_MODE=smtp`. SMTP mode requires:

- `SMTP_ADDR` such as `smtp.example.com:587`
- `SMTP_HOST` TLS server name
- `SMTP_FROM` envelope/from address
- optional `SMTP_USERNAME`
- optional `SMTP_PASSWORD`

SMTP delivery requires STARTTLS with TLS 1.2 or newer.

## Audit log

Workspace audit records are stored durably in PostgreSQL. Current audited events include registration/workspace creation, login, email verification, membership changes, API-key creation, and owned-link creation.

Workspace owners/admins can read the latest audit entries at:

```text
GET /api/v1/workspaces/{workspace}/audit
```
