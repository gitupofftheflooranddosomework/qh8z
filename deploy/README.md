# qh8z production deployment

The reference production target for the initial qh8z launch is a single Canadian VPS running Ubuntu 24.04 LTS and Docker Engine with the Compose plugin. The selected baseline is an OVHcloud Canada VPS-2 class host (4 vCPU, 8 GB RAM, 75 GB NVMe or better). The Compose stack remains portable to another VPS with equivalent resources.

The public attack surface is intentionally small:

- Caddy: public TCP 80/443 and UDP 443
- Prometheus: loopback `127.0.0.1:9090` only
- Alertmanager: loopback `127.0.0.1:9093` only
- qh8z: Docker networks only
- PostgreSQL: internal Docker network only
- backup service: internal Docker network only

## 1. Host preparation

Install Docker Engine and the Docker Compose plugin from Docker's supported Ubuntu repository. Create `/opt/qh8z` for the repository checkout and `/etc/qh8z/secrets` for secrets.

Firewall policy:

- allow TCP 80 and 443 from the Internet
- allow UDP 443 from the Internet for HTTP/3
- restrict SSH to trusted administrator IPs or a VPN
- do not expose 5432, 8080, 9090, or 9093 publicly

## 2. DNS

Point the apex `qh8z.com` A/AAAA records at the VPS and point `www.qh8z.com` at the same service. Caddy obtains and renews the primary certificates automatically.

Customer custom domains must point at qh8z and complete the TXT verification shown by the dashboard. Caddy's on-demand TLS `ask` endpoint calls qh8z internally and authorizes certificate issuance only for hosts that are already verified in the `custom_domains` table.

## 3. Configuration

Copy the example environment file:

```bash
cp deploy/.env.production.example deploy/.env.production
chmod 600 deploy/.env.production
```

Edit the non-secret values in that file.

Generate qh8z-owned secrets:

```bash
sudo QH8Z_SECRETS_DIR=/etc/qh8z/secrets deploy/init-secrets.sh
```

Then install these service-provided secrets as root-readable files with mode `0600`:

```text
/etc/qh8z/secrets/smtp_password
/etc/qh8z/secrets/webrisk_api_key
/etc/qh8z/secrets/stripe_secret_key
/etc/qh8z/secrets/stripe_webhook_secret
/etc/qh8z/secrets/restic_s3_access_key
/etc/qh8z/secrets/restic_s3_secret_key
/etc/qh8z/secrets/alertmanager_discord_webhook
```

The generated qh8z-owned secret files are:

```text
/etc/qh8z/secrets/postgres_password
/etc/qh8z/secrets/admin_token
/etc/qh8z/secrets/rate_limit_salt
/etc/qh8z/secrets/restic_password
```

Never place these values in Git.

## 4. Validate before deployment

From the repository root:

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml config --quiet
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml build qh8z backup
```

Take an encrypted one-shot database backup before replacing an existing release:

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml run --rm -e BACKUP_ONCE=1 backup
```

## 5. Deploy

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml up -d --build --remove-orphans
```

Confirm:

```bash
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml ps
curl -fsS https://qh8z.com/healthz
```

The qh8z container's Docker health check calls `/readyz`, so it becomes healthy only when the application can reach PostgreSQL.

## 6. Observability

The application exposes Prometheus-format metrics internally at `/metrics`. Caddy explicitly blocks `/metrics`, `/readyz`, and `/internal/*` from public access.

Prometheus evaluates the repository alert rules and sends notifications through Alertmanager. The reference receiver is a Discord incoming webhook stored as a Docker secret. Access the monitoring UIs from an SSH tunnel rather than opening public firewall ports:

```bash
ssh -L 9090:127.0.0.1:9090 -L 9093:127.0.0.1:9093 <server>
```

Then browse to local ports 9090 and 9093.

## 7. Backups

The `backup` service runs `pg_dump` and stores the dump in an encrypted restic repository every six hours by default. It keeps 24 hourly, 14 daily, 8 weekly, and 12 monthly restore points.

The backup repository must be off-host object storage. VPS snapshots are useful additional protection but do not replace the application-level offsite backup.

See [`../docs/PRODUCTION_OPERATIONS.md`](../docs/PRODUCTION_OPERATIONS.md) for restore, incident, rollback, and failure-test procedures.
