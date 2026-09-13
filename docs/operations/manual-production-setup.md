# Manual first production deployment

Prepared 2026-09-13 against `3d47ff8`. This is an operator procedure, not evidence
that a production deployment has happened. Use this guide for the manual VM
deployment; the GitHub **CD · prod** workflow still targets Cloudflare.
After initial setup, use [Deploy latest HEAD](deploy-latest.md) for repeat releases.

For the requested **Nginx + Cloudflare + HTTPS admin hostname** variant, use the
[bootstrap script and Nginx guide](nginx-cloudflare-setup.md). It replaces the
Caddy/SSH-only proxy steps below; the backup and recovery requirements still apply.

The assumed host is the previously selected **8 vCPU / 24 GB RAM / 200 GB NVMe,
Ubuntu 26.04** machine. A VPC is the private network; these commands run on the
VM/VPS inside it. The VM needs public HTTPS ingress and outbound Internet access.
Smaller machines need a separately reviewed resource configuration.

The stack is Caddy → Marum → PostgreSQL. Marum contains the Telegram bot,
embedded Mini App and private admin UI. Telegram uses outbound long polling;
there is no webhook service, Redis/Valkey, or separate frontend container.
Keep one Marum instance per bot token. The existing in-memory planner cache is
sufficient for the initial deployment; measure before adding a cache service.

## 1. Collect the deployment inputs

Have these ready before touching the server:

- VM IP, SSH login with sudo, SSH key, and provider recovery-console access.
- Public hostname, such as `marum.loan`, and an email for certificate notices.
- A dedicated production bot created through Telegram's BotFather and its token.
- An immutable reviewed Git tag/commit containing the production preparation.
  `v1.0.0` already exists and predates this work; do not deploy it or overwrite it
  merely because this is your first production launch. Choose a new version.
- An independent backup destination, credentials, and a password manager to hold
  the identity key and backup encryption password separately from the VM.
- An external monitoring destination and an operator to receive alerts.
- Whether production starts empty or imports existing accounts. The numbered
  install path starts **empty**. For imports, use section 12 before starting Marum.

Keep the public launch closed to borrowers until section 10 passes. The app
starts polling as soon as its container starts, even before Caddy starts.

## 2. Configure DNS and the network

Create an A record for the hostname pointing to the VM. Add AAAA only if IPv6
routing and firewall rules work. If your DNS is hosted in Cloudflare, use
DNS-only for this direct-Caddy deployment.

Use the provider firewall/security group for inbound rules:

| Port | Source | Purpose |
| --- | --- | --- |
| TCP 22, or your actual SSH port | Operator IP/CIDR only | SSH |
| TCP 80 | Internet | HTTP redirect and certificate validation |
| TCP 443 | Internet | Mini App HTTPS |
| UDP 443 | Internet, optional | HTTP/3 |
| Everything else | Deny | Database and admin stay private |

Permit outbound DNS, NTP and HTTPS, including Telegram, registries, ACME,
telemetry and backup storage. Apply equivalent rules to IPv6. If the VM has no
public route, provision a suitable public ingress/NAT design first; this guide
assumes Caddy terminates public TLS on the VM.

Docker-published ports can bypass UFW rules. The Compose file publishes app/admin
ports only on `127.0.0.1` and does not publish PostgreSQL. Preserve those bindings
and use the provider firewall as well. See [Docker's firewall limitations](https://docs.docker.com/engine/install/ubuntu/#firewall-limitations).

Confirm a second SSH session works before closing your original connection.
Use SSH key authentication; disable password/root SSH login only after verifying
your sudo-enabled account and provider console access.

## 3. Prepare Ubuntu and Docker

SSH into the VM. All server commands below use a root Bash session unless
explicitly marked **workstation**. Do not enable shell tracing (`set -x`).

```bash
sudo -i
umask 077
apt-get update
apt-get upgrade -y
apt-get install -y ca-certificates curl git python3 openssl make \
  restic util-linux dnsutils nano unattended-upgrades
timedatectl status
df -h /
free -h
nproc
```

Reboot now if `/var/run/reboot-required` exists; reconnect and run `sudo -i`
again. Confirm the clock is synchronized, especially before admin TOTP use.
Configure automatic security updates and schedule required reboots.

On a fresh host, install Docker Engine from its official apt repository:

```bash
install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat > /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
docker run --rm hello-world
docker compose version
```

If Docker/containerd is already installed, resolve package conflicts using the
[official Ubuntu installation instructions](https://docs.docker.com/engine/install/ubuntu/)
before continuing. Use the `docker compose` plugin, not the legacy Python command.
Do not expose the Docker daemon over an unauthenticated TCP socket.

## 4. Check out the release and define the production command

Replace `YOUR_REVIEWED_TAG_OR_COMMIT` before running. For a private repository,
use a read-only deploy key or another authorized Git credential; do not embed a
token in the clone URL. If `/opt/marum` already exists, inspect it instead of
cloning over it.

```bash
git clone https://github.com/andranikasd/marumbot.git /opt/marum
cd /opt/marum
git fetch --tags origin
git checkout --detach YOUR_REVIEWED_TAG_OR_COMMIT
git status --short
git rev-parse HEAD
install -d -m 0700 /etc/marum /var/backups/marum /var/lib/marum/release-records

dc() {
  docker compose --env-file /etc/marum/compose.env \
    -f /opt/marum/deploy/vps/compose.yml "$@"
}
```

Re-enter that `dc` function after reconnecting. It fixes both the production
Compose file and secret-file location. Do not use plain `docker compose up`,
`make up`, `make reset`, or `docker compose down -v` for this installation.

## 5. Create production configuration outside the checkout

The current `.dockerignore` does not exclude nested `.env` files. This procedure
avoids that issue by keeping **all real secrets outside `/opt/marum`**. Do not
create `deploy/vps/.env`. Check a reused checkout for secret files before building.
Recursive Docker ignore hardening remains a recommended repository fix.

For a new, empty production database only, generate independent secrets directly
into a new root-readable file. This command refuses to overwrite an existing file:

```bash
cd /opt/marum
python3 - <<'PY'
import base64
import os
import secrets
from pathlib import Path

text = Path('deploy/vps/.env.example').read_text()
values = {
    'POSTGRES_PASSWORD': secrets.token_hex(32),
    'MARUM_DB_PASSWORD': secrets.token_hex(32),
    'MARUM_SERVICE_TOKEN': secrets.token_hex(32),
    'MARUM_IDENTITY_KEY': base64.b64encode(secrets.token_bytes(32)).decode(),
}
lines = []
for line in text.splitlines():
    key = line.partition('=')[0]
    lines.append(key + '=' + values[key] if key in values else line)
fd = os.open('/etc/marum/compose.env', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as out:
    out.write('\n'.join(lines) + '\n')
PY
nano /etc/marum/compose.env
```

Set these remaining values:

| Variable | Value |
| --- | --- |
| `MARUM_BOT_TOKEN` | Production BotFather token |
| `MARUM_DOMAIN` | Hostname only, no scheme or slash |
| `ACME_EMAIL` | Certificate contact email |
| `MARUM_VERSION` | Unique release version corresponding to the checkout |
| `MARUM_ADMIN_USER` | Chosen bootstrap admin username |
| `MARUM_ADMIN_PASSWORD_HASH` | Optional; add after building below |
| `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS` | Your configured telemetry destination; see the Grafana guide |

Keep the resource defaults for the specified 24-GB host. Keep `MARUM_IMAGE` and
`MARUM_MIGRATE_IMAGE` empty for this first local build; PostgreSQL/Caddy use the
reviewed pinned defaults. Registry images by digest are preferable for later
releases. The migrator still mounts migrations from this exact checkout.

Back up the identity key in your password manager now. For imported data, use
the **original** identity key; a new key cannot decrypt old identities or admin
credentials. Never regenerate secrets during an upgrade. Editing the password
variables does not rotate passwords in an existing PostgreSQL volume.

Create the journal directory for a fresh install:

```bash
install -d -m 0700 /var/lib/marum/erasures
python3 -c "import os; os.chown('/var/lib/marum/erasures', 65532, 65532)"
dc config --quiet
```

An empty journal is valid only for a new installation. Recovery requires the
latest authoritative journal. Keep it outside the database volume. Use only
`config --quiet` in shared output: expanded Compose configuration contains secrets.

## 6. Build and optionally configure admin

The initial build can run on the VM before it has production traffic. For future
upgrades, build on a separate machine/CI and transfer or pull immutable images.

```bash
cd /opt/marum
dc pull postgres caddy
dc build --pull marum migrate
```

If a build fails, stop and resolve it before starting the service. Do not replace
a pinned image with an arbitrary version to get past a pull/build failure.

Optional admin setup: prompt for a password without echoing it or placing it in
shell history, and hash it using the built application:

```bash
read -r -s -p 'New bootstrap admin password: ' marum_admin_password
printf '\n'
printf '%s\n' "$marum_admin_password" | \
  dc run --rm --no-deps -T marum -hash-password
unset marum_admin_password
nano /etc/marum/compose.env
```

Paste the resulting hash as `MARUM_ADMIN_PASSWORD_HASH='HASH_HERE'` with single
quotes to preserve dollar signs. If left empty, the admin listener is disabled.
The hash seeds an admin only when the identity registry is empty; it does not
reset an existing account. First login requires authenticator enrollment.

## 7. Cut over Telegram and start the stack

For an existing bot, stop its old polling/webhook deployment and scheduler first.
Keep pending updates. Telegram does not permit polling while a webhook remains
configured; see [deleteWebhook and getWebhookInfo](https://core.telegram.org/bots/api#deletewebhook).

Run this once, using the production token at the hidden prompt. It prints only
success markers, not the token or bot identity. It deliberately removes the
webhook **without discarding pending updates**:

```bash
python3 - <<'PY'
import getpass
import json
import urllib.parse
import urllib.request

token = getpass.getpass('Production bot token: ')
def call(method, fields=None):
    data = urllib.parse.urlencode(fields or {}).encode()
    try:
        with urllib.request.urlopen(
            'https://api.telegram.org/bot' + token + '/' + method,
            data=data, timeout=20,
        ) as response:
            result = json.load(response)
    except Exception:
        raise SystemExit('Telegram request failed; verify token/network without logging the URL.') from None
    if not result.get('ok'):
        raise SystemExit('Telegram rejected the operation.')
    return result['result']
call('getMe')
call('deleteWebhook', {'drop_pending_updates': 'false'})
if call('getWebhookInfo').get('url'):
    raise SystemExit('Webhook still configured; do not start polling.')
print('Token accepted; webhook removed; pending updates preserved.')
PY
dc config --quiet
dc up -d --no-build --wait --wait-timeout 180
dc ps --all
```

Expect PostgreSQL, Marum and Caddy running; the one-shot `migrate` service exits
successfully with code 0. Compose waits for PostgreSQL health, then migrations,
then app health before starting Caddy. A failed migration is a stop condition.
Do not bypass it with `--no-deps` on first startup.

## 8. Verify the deployment and admin access

On the **server**, set the nonsecret hostname and inspect the service:

```bash
MARUM_PUBLIC_DOMAIN='YOUR_DOMAIN'
dig +short A "$MARUM_PUBLIC_DOMAIN"
dig +short AAAA "$MARUM_PUBLIC_DOMAIN"
curl --fail --max-time 5 http://127.0.0.1:8080/healthz
curl --fail --max-time 5 http://127.0.0.1:8080/readyz
curl --fail --max-time 5 http://127.0.0.1:8080/status
curl --fail --max-time 20 -o /dev/null "https://$MARUM_PUBLIC_DOMAIN/app/"
dc logs --tail=100 marum caddy
docker stats --no-stream
ss -lnt
```

`/readyz` must show `database: true`, the intended build version and schema 26
for this reviewed checkout. Container health alone checks liveness and does not
prove database readiness or Telegram delivery. Caddy must obtain a trusted
certificate; do not use `curl -k` to make the check pass.

From a **workstation/external network**, verify these expected status codes:

```bash
MARUM_PUBLIC_DOMAIN='YOUR_DOMAIN'
curl -sS -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/app/"           # 200
curl -sS -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/app/api/loans"  # 401
curl -sS -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/readyz"         # 404
curl -sS -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/status"         # 404
curl -sS -X POST -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/internal/tick" # 404
curl -sS -X POST -o /dev/null -w '%{http_code}\n' "https://$MARUM_PUBLIC_DOMAIN/tg/update"     # 404
```

Check TCP 5432, 8080 and 8081 are unreachable externally, including via IPv6.
For admin, run on your **workstation** and keep the tunnel open:

```bash
ssh -N -L 8081:127.0.0.1:8081 OPERATOR@VM_IP
```

Open `http://127.0.0.1:8081`, enroll TOTP, and verify a complete login. Keep the
listener private. The bootstrap administrator does not automatically have
financial-review roles; set up role separation through the admin security UI.
See [admin security integration](../../internal/adapter/in/admin/README.md).

Record the deployed identity on the **server**:

```bash
cd /opt/marum
record="/var/lib/marum/release-records/$(date -u +%Y%m%dT%H%M%SZ).txt"
{
  git rev-parse HEAD
  dc images
  docker inspect --format '{{.Image}}' "$(dc ps -q marum)"
  curl --fail --max-time 5 http://127.0.0.1:8080/readyz
} > "$record"
```

## 9. Enable independent encrypted backups and rehearse recovery

Create a private bucket/repository on storage independent of the VM. The example
uses S3-compatible storage; substitute your provider's actual endpoint and region.
Credentials need the operations used by restic backup, restore and retention.
Enable provider protection against loss of the whole account where available.
Do not apply an unrelated object-expiration policy to restic's internal objects.

Create a backup encryption password once and preserve it independently:

```bash
test ! -e /etc/marum/restic-password && \
  (umask 077; openssl rand -base64 48 > /etc/marum/restic-password)
nano /etc/marum/backup.env
```

Put the following in `/etc/marum/backup.env`, replacing all placeholders.
Use simple `KEY=value` lines with no `export`; single-quote values needing quotes.
These examples are compatible with systemd and the root shell below.

```dotenv
RESTIC_REPOSITORY=s3:https://YOUR_S3_ENDPOINT/YOUR_BUCKET/marum-production
RESTIC_PASSWORD_FILE=/etc/marum/restic-password
AWS_ACCESS_KEY_ID=YOUR_BACKUP_ACCESS_KEY
AWS_SECRET_ACCESS_KEY=YOUR_BACKUP_SECRET_KEY
AWS_DEFAULT_REGION=YOUR_REGION
MARUM_COMPOSE_ENV=/etc/marum/compose.env
MARUM_BACKUP_DIR=/var/backups/marum
MARUM_BACKUP_LOCK=/run/lock/marum-backup.lock
MARUM_BACKUP_RETENTION_DAYS=14
MARUM_RESTIC_KEEP_DAILY=14
MARUM_RESTIC_KEEP_WEEKLY=8
MARUM_RESTIC_KEEP_MONTHLY=12
```

Initialize a **new** restic repository, then install the supplied units:

```bash
chmod 600 /etc/marum/backup.env /etc/marum/restic-password
(
  set -a
  . /etc/marum/backup.env
  set +a
  restic init
)
cp /opt/marum/deploy/vps/marum-backup.service /etc/systemd/system/
cp /opt/marum/deploy/vps/marum-backup.timer /etc/systemd/system/
systemctl daemon-reload
systemctl start marum-backup.service
systemctl enable --now marum-backup.timer
systemctl list-timers marum-backup.timer
journalctl -u marum-backup.service --since today --no-pager
```

An existing repository uses its existing password: check with `restic snapshots`
instead of initializing it again. The timer runs daily at 03:15 UTC plus up to
15 minutes of jitter. A manual `systemctl start marum-backup.service` uses the
same environment and lock as scheduled backups. Check exit status, not just the
presence of a local dump. Setup details: [restic repositories](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html).

Before public launch, create a fixture-backed approved plan and enroll an admin
on a controlled test account, then take another backup. An empty-database restore
does not prove encrypted credentials and historical plans can be recovered.
Download the latest snapshot into a **new** directory:

```bash
restore_dir=$(mktemp -d /var/backups/marum-restore-XXXXXX)
export restore_dir
(
  set -a
  . /etc/marum/backup.env
  set +a
  restic snapshots --host "$(hostname)" --tag marum-postgres
  restic check
  restic restore latest --host "$(hostname)" --tag marum-postgres --target "$restore_dir"
)
find "$restore_dir" -name database.dump -type f
```

Select the printed dump path and run the existing isolated restore checker using
the actual deployed application/PostgreSQL image IDs and the recovered journal:

```bash
export MARUM_VERIFY_IMAGE=$(docker inspect --format '{{.Image}}' "$(dc ps -q marum)")
export MARUM_POSTGRES_IMAGE=$(docker inspect --format '{{.Image}}' "$(dc ps -q postgres)")
export MARUM_ERASURE_JOURNAL_DIR="$restore_dir/var/lib/marum/erasures"
# For this prelaunch rehearsal, keep deletion activity paused so the snapshot
# journal is authoritative. Never assume an older snapshot journal is current.
test -d "$MARUM_ERASURE_JOURNAL_DIR"
chown -R 65532:65532 "$MARUM_ERASURE_JOURNAL_DIR"
chmod 700 "$MARUM_ERASURE_JOURNAL_DIR"
read -r -s -p 'Original MARUM_IDENTITY_KEY from password manager: ' MARUM_IDENTITY_KEY
printf '\n'
export MARUM_IDENTITY_KEY
sh /opt/marum/deploy/vps/restore-check.sh /ABSOLUTE/RECOVERED/PATH/database.dump
unset MARUM_IDENTITY_KEY MARUM_VERIFY_IMAGE MARUM_POSTGRES_IMAGE MARUM_ERASURE_JOURNAL_DIR
```

Expected: checksum, restore, restricted grants, ledger guards, identity/admin
decryption, and original-plan replay all pass. The checker removes its disposable
database; the downloaded recovery files remain private until you remove them.
It may reconcile the supplied journal, so use the recovered copy for this drill.
See [restic restore](https://restic.readthedocs.io/en/stable/050_restore.html).

Preserve the identity key, restic password, configuration and release images in
independent secure storage. The supplied backup script backs up the database and
journal, **not** `/etc/marum` or Docker images. A provider VM snapshot supplements
this backup; it does not replace the verified restore.

Daily dumps imply about one day of potential financial-data loss. If that is
unacceptable, implement and test WAL archiving/PITR before launch. A journal on
the same VM is independent of the database volume, but not of host failure:
daily uploads do not preserve every later deletion intent. Establish an
independent, sufficiently current deletion record; after host loss, keep restored
accounts offline whenever journal freshness cannot be established.

## 10. Wire alerts and complete the launch checks

Configure the existing OTLP exporter using [Grafana Cloud](grafana-cloud.md) or
your chosen compatible destination. Recreate Marum after changing exporter
settings (`dc up -d --no-build --no-deps marum`). Keep health/status probes private;
collect them from a host agent or over a secure operator connection.

These are initial alert policies, not measured capacity guarantees:

| Signal | Initial action/threshold |
| --- | --- |
| External HTTPS `/app/` | Alert after sustained failure, e.g. 2 minutes |
| Private `/readyz` | Alert on database/schema failure |
| Backup service | Alert on failure; test the notification route |
| Offsite snapshot age | Alert if newest successful snapshot exceeds 26 hours |
| Disk space/inodes | Alert before less than 20% free |
| OOM, restart loops, unhealthy app | Alert and inspect readiness/logs |
| Inbox/reminder age | Alert on sustained growth or missed delivery target |
| Planner latency, HTTP errors, DB pool waits | Establish baseline with authenticated representative load |

Installing the backup timer does **not** install an alert receiver. Verify a
synthetic alert reaches you, and that the external monitor detects a planned
prelaunch outage. Docker restart policy restarts exited processes; it does not
automatically repair an unhealthy-but-running process.

Before announcing production:

- Open the Mini App from the production bot on a real phone; test both languages.
- Use controlled, fixture-backed inputs to create/edit loans and budgets, approve
  a plan, record/correct a payment, and reconcile. Check expected fixture results.
- Verify inbox processing, reminder delivery, restart behavior, and admin TOTP.
- Run authenticated representative concurrent reads/writes. The existing k6
  smoke profile tests probes only and does not establish planning capacity.
- Verify a reboot before public traffic: containers restart, readiness returns,
  HTTPS works, and the backup timer is scheduled. Never rerun first-time init.
- Confirm offsite restore, alert delivery, preserved keys/journal, and the
  immutable release record. Keep the old bot deployment stopped.

## 11. Routine operations, upgrades and rollback

Useful server commands, after restoring the `dc` function:

```bash
dc ps --all
dc logs --tail=100 marum
dc logs --tail=100 postgres
curl --fail --max-time 5 http://127.0.0.1:8080/readyz
systemctl start marum-backup.service
systemctl list-timers marum-backup.timer
df -h
df -i
docker stats --no-stream
```

For an upgrade, prebuild/pull reviewed images and prove previous-binary/schema
compatibility first. Schedule a maintenance window. Stop Marum and take a final
backup so the checkpoint includes the last accepted writes. Pause the backup
timer while changing the release checkout; wait for any active backup to finish.

1. Record current commit, images and config in private release records. Preserve
   the current image and a secure copy of `/etc/marum/compose.env` outside the repo.
2. `dc stop marum`, then `systemctl start marum-backup.service`; check success.
3. Stop the timer, switch `/opt/marum` to the reviewed new tag, and update only
   version/image settings in `/etc/marum/compose.env`. Preserve secrets and volumes.
4. Pull the new images by digest. With local builds, build only during the
   maintenance window; do not rebuild under live traffic or reuse old image tags.
5. `dc config --quiet`, then `dc run --rm migrate`. If migration fails, leave
   Marum stopped and investigate; do not bypass the gate.
6. `dc up -d --no-build --wait --wait-timeout 180`. Repeat readiness, HTTPS,
   Telegram and fixture smoke checks. Restart `marum-backup.timer` and verify it.

If a compatible previous image is available, set `MARUM_IMAGE` to its retained
digest/tag and use `dc up -d --no-build --no-deps marum`. Verify the actual build
version afterwards. Keep the expanded schema; do not automatically migrate down.
Changing `MARUM_VERSION` alone cannot change a compiled binary's version.

**Pre-hardening releases cannot read the newly encrypted admin TOTP credentials.**
Do not treat historical `v2.0.4` as a proven rollback image for this transition.
The first production release needs a compatible repair/roll-forward strategy.
Restoring a database backup is disaster recovery with possible data loss, not a
routine application rollback. See [VPS rollback constraints](vps.md#releases-and-rollback).

## 12. Import or disaster recovery onto a fresh host

Use this only on a new, isolated target with an empty production database volume.
Keep the old deployment and all app processes stopped during final migration.
For an existing installation, do not delete or overwrite its volume to make the
following commands work. Prepare a new target instead.

1. Complete host/Docker/checkout setup. Restore configuration securely, retain the
   original identity key, and use database passwords consistent with the new
   role bootstrap. Bring back the exact compatible release image and migration
   checkout. Recover the verified custom-format dump/checksum.
2. Restore the latest authoritative erasure journal into
   `/var/lib/marum/erasures`, owned by `65532:65532`, directory mode `0700`.
   If freshness is unknown after total host loss, stop here until reconciled.
3. Start **only** PostgreSQL, restore before applying migrations, then reapply
   restricted grants. Set `recovery_dump` to the recovered file:

```bash
recovery_dump='/ABSOLUTE/RECOVERED/PATH/database.dump'
(cd "$(dirname "$recovery_dump")" && sha256sum -c "$(basename "$recovery_dump").sha256")
dc up -d --no-build --wait postgres
dc exec -T postgres pg_restore -U marum_owner -d marum \
  --exit-on-error --single-transaction --no-owner --no-acl < "$recovery_dump"
dc run --rm migrate
dc exec -T postgres psql -v ON_ERROR_STOP=1 -U marum_owner -d marum \
  < /opt/marum/queries/provisioning/restrict-ledgers.sql
dc run --rm --no-deps marum -verify-restore
```

4. Only after verification succeeds, perform Telegram cutover and start the
   stack as in section 7. Verify section 8, take a new offsite backup, and confirm
   monitoring before reopening accounts. Do not import development/demo accounts
   into production unless that migration is intentional.

## 13. Troubleshooting

| Symptom | Check |
| --- | --- |
| Missing journal bind directory | Provision the path/ownership in section 5; for recovery, restore authoritative contents |
| Database authentication failure | Existing role passwords do not follow edited env values; coordinate role rotation, never delete the volume |
| Migrator fails | Inspect `dc logs migrate`; keep Marum stopped and investigate the actual SQL error |
| Caddy cannot get TLS | A/AAAA records, inbound 80/443, outbound DNS/HTTPS, conflicting listeners, Caddy logs |
| Bot is silent but HTTP works | Token, old deployment/webhook conflict, Telegram egress, inbox/reminder status |
| Admin disabled | Empty bootstrap hash; after setting it, recreate the app |
| Admin login/TOTP fails | Correct identity key, synchronized clock, enrolled authenticator; env password changes do not reset stored identities |
| Backup fails | `journalctl -u marum-backup.service`; correct `MARUM_COMPOSE_ENV`, mount, storage credentials, restic password, disk |
| Restore verification fails | Correct key/journal/image, compatible historical engine, intact dump; do not open ingress |

Keep diagnostics private and avoid printing full container environments, expanded
Compose files, tokens, borrower identifiers, financial values or raw SQL parameters.
