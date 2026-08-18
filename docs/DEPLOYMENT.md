# QH8Z production deployment

QH8Z includes a manual GitHub Actions workflow named **Deploy production** plus host-side `scripts/deploy.sh` and `scripts/rollback.sh` guardrails.

## One-time host setup

1. Provision the production host as described in `docs/LAUNCH.md`.
2. Clone this public repository into a dedicated deployment directory owned by the deployment user.
3. Create the production `.env` from `.env.production.example`; keep `.env` on the host and out of Git. Generate independent 32-byte hex values for the PostgreSQL maintenance password, QH8Z database-role password, Shlink database-role password, Shlink API key, MFA encryption key, and initial bootstrap secret.
4. Run `bash scripts/preflight.sh .env`, start the stack, bootstrap the administrator, enroll admin MFA, and run `bash scripts/postdeploy.sh https://qh8z.com` manually once.
5. Configure the GitHub `production` environment with required reviewers if desired.

The PostgreSQL `postgres` credential is maintenance-only. The app receives only `QH8Z_DB_PASSWORD` and connects as `qh8z_app`; Shlink receives only `SHLINK_DB_PASSWORD` and connects as `shlink_app`. Do not collapse these credentials into one password.

## GitHub production environment secrets

The `Deploy production` workflow requires:

- `DEPLOY_HOST`: production SSH hostname or IP.
- `DEPLOY_USER`: unprivileged deployment account with access to the QH8Z checkout and Docker.
- `DEPLOY_SSH_KEY`: private SSH key dedicated to deployment.
- `DEPLOY_KNOWN_HOSTS`: pinned `known_hosts` line(s) for the production server; do not use `StrictHostKeyChecking=no`.
- `DEPLOY_PATH`: absolute path to the QH8Z checkout on the production host.

The deployment account should not be a general interactive administrator. Limit the SSH key and host permissions to what QH8Z deployment requires.

## Deploy behavior

The workflow is manual (`workflow_dispatch`) and defaults to deploying `origin/main`. GitHub Actions uses only the built-in SSH client; it does not add a third-party deployment action.

On the host, `scripts/deploy.sh`:

1. refuses tracked local modifications;
2. fetches the requested Git ref;
3. creates a verified two-database backup using the selected production env file before changing code;
4. checks out the exact target commit;
5. runs production preflight against the host `.env`;
6. pulls/builds and starts the production Compose stack in one production-profile transaction;
7. waits for local `/readyz` with bounded connection/response timeouts;
8. runs bounded public `scripts/postdeploy.sh` checks, including exact `/rest` and `/rest/*` management isolation;
9. automatically returns to the previous application commit if deployment fails, while retaining the pre-deploy database backup.

A deploy rollback intentionally does **not** automatically rewind database contents. Current migrations are designed to be additive, and automatically replacing live post-deploy data would be more dangerous than leaving the verified pre-deploy backup available for an explicit recovery decision.

## Backup/recovery behavior

`backup.sh` quiesces Caddy, QH8Z, and Shlink long enough to take consistent custom-format dumps of both application databases with the maintenance role. It then restores exactly the service set that was previously running with `--no-recreate`. The command exits nonzero if it cannot return those services after the snapshot, even if the archive itself was written successfully.

`restore.sh` verifies the archive/checksum, recreates `qh8z` and `shlink` with the correct `qh8z_app`/`shlink_app` owners, reapplies database CONNECT restrictions, restores data as the corresponding application role, and then reopens only the service set that was running before recovery. If restore fails before completion, application/redirect services remain stopped for inspection rather than serving a partial restore.

The default local `backups/` directory is gitignored. Production backups should still be encrypted and copied off-host.

## Manual code rollback

```bash
bash scripts/rollback.sh <known-good-commit-or-ref> .env
```

The rollback command creates another backup with the selected env file before switching code, rebuilds the stack, waits for readiness with bounded probes, and runs public post-deploy checks. Use `scripts/restore.sh` separately only when a database recovery is actually required.
