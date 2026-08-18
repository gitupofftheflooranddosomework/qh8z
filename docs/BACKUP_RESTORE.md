# PostgreSQL backup and restore runbook

This runbook covers the manual recovery procedure for qh8z PostgreSQL data. Automated backups, retention, off-site replication, and restore drills remain part of the production-operations gate.

## Backup

1. Obtain a read-capable production database connection from the secrets manager. Do not commit database credentials or paste them into tickets, logs, or chat.
2. Create a custom-format PostgreSQL dump:

   ```bash
   pg_dump --format=custom --compress=9 --no-owner --no-acl --dbname="$DATABASE_URL" --file="qh8z-$(date -u +%Y%m%dT%H%M%SZ).dump"
   ```

3. Generate a SHA-256 checksum and store it next to the backup:

   ```bash
   sha256sum qh8z-*.dump > qh8z-backup.sha256
   ```

4. Move the dump and checksum to encrypted, access-controlled backup storage. Production automation should make this storage separate from the primary database account/project.

## Restore drill

Always restore into an isolated database first. Never test a restore over the live production database.

1. Provision an empty PostgreSQL database of a supported version.
2. Restore the dump:

   ```bash
   pg_restore --no-owner --no-acl --exit-on-error --dbname="$RESTORE_DATABASE_URL" qh8z-YYYYMMDDTHHMMSSZ.dump
   ```

3. Verify the migration ledger and row counts:

   ```bash
   psql "$RESTORE_DATABASE_URL" -c 'TABLE schema_migrations;'
   psql "$RESTORE_DATABASE_URL" -c 'SELECT count(*) AS links FROM links;'
   psql "$RESTORE_DATABASE_URL" -c 'SELECT count(*) AS visits FROM visits;'
   ```

4. Start qh8z against the restored database with `QH8Z_STORAGE=postgres` and confirm `/readyz` returns HTTP 200.
5. Test several known short links and their `/api/v1/links/{slug}/stats` endpoints.

## Disaster recovery

For a real incident, stop writes or switch traffic away from the damaged database, restore the newest verified backup into a replacement database, run the verification steps above, then update the production database secret and redeploy. Do not point production at a restore until link/visit counts, migrations, readiness, and representative redirects have been verified.

## Launch follow-up

Before public launch, Gate 5 must add automatic scheduled backups, retention policy, encryption verification, monitoring for failed backups, and recurring restore drills with documented recovery-time and recovery-point objectives.
