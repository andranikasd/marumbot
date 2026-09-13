#!/usr/bin/env bash
# Fresh Ubuntu host, empty production database only. Run a saved file, not curl|bash.
# Usage: sudo bash bootstrap-ubuntu.sh TAG_OR_FULL_COMMIT [--nginx] [--cloudflare] [--take-over-bot]
# Docker installation: https://docs.docker.com/engine/install/ubuntu/
set +x
set -Eeuo pipefail
umask 077

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
trap 'printf "Setup stopped at line %s. Existing files, secrets and volumes were preserved. Diagnose before retrying; do not delete database volumes.\n" "$LINENO" >&2' ERR

[[ $EUID -eq 0 ]] || die 'Run with sudo bash.'
[[ $# -ge 1 ]] || die 'Usage: bootstrap-ubuntu.sh TAG_OR_FULL_COMMIT [--nginx] [--take-over-bot]'
release_ref=$1
shift
take_over=0
use_nginx=0
use_cloudflare=0
resume_before_config=0
for option in "$@"; do
  case "$option" in
    --take-over-bot) take_over=1 ;;
    --nginx) use_nginx=1 ;;
    --cloudflare) use_cloudflare=1; use_nginx=1 ;;
    --resume-before-config) resume_before_config=1 ;;
    *) die 'Unknown option.' ;;
  esac
done
[[ $release_ref =~ ^[a-fA-F0-9]{40}$ || $release_ref =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] ||
  die 'Choose a version tag or full 40-character commit, not a moving branch.'
[[ -r /etc/os-release ]] || die 'Cannot identify operating system.'
# shellcheck disable=SC1091
. /etc/os-release
[[ $ID == ubuntu ]] || die 'This script requires Ubuntu.'
case "$VERSION_ID" in
  22.04|24.04|26.04) ;;
  *) die 'Use a Docker-supported Ubuntu LTS: 22.04, 24.04 or 26.04.' ;;
esac
[[ -t 0 ]] || die 'Run interactively from a saved script; prompts need a terminal.'
if (( resume_before_config )); then
  [[ -d /opt/marum/.git && ! -L /opt/marum && ! -L /etc/marum ]] || die 'Resume requires the existing real checkout.'
  [[ ! -e /etc/marum/compose.env && ! -L /etc/marum/compose.env ]] || die 'Configuration already exists; do not regenerate secrets.'
  [[ ! -e /usr/local/sbin/marum-compose ]] || die 'Setup progressed past configuration; use the manual guide.'
  [[ ! -L /var/lib/marum/erasures ]] || die 'Journal must not be a symlink.'
  if [[ -d /var/lib/marum/erasures ]]; then
    [[ -z $(find /var/lib/marum/erasures -mindepth 1 -print -quit) ]] || die 'Journal is not empty; this is not a fresh setup.'
  fi
  [[ -z $(git -C /opt/marum status --porcelain) ]] || die 'Checkout has local modifications; inspect before resuming.'
else
  for location in /opt/marum /etc/marum /var/lib/marum/erasures /usr/local/sbin/marum-compose; do
    [[ ! -e $location && ! -L $location ]] || die "Existing installation path: $location. Use the manual guide to resume or upgrade."
  done
fi
# The checked-in resource envelope is for the selected 8-vCPU / 24-GB VM.
memory_kib=$(awk '/MemTotal:/ {print $2}' /proc/meminfo)
(( memory_kib >= 22000000 && $(nproc) >= 8 )) ||
  die 'This Compose preset needs the selected 8-vCPU/24-GB host. Review limits for a smaller machine first.'
available_kib=$(df --output=avail / | tail -1 | tr -d ' ')
(( available_kib >= 40000000 )) || die 'Keep at least 40 GB free for the first build and database.'

printf '%s\n' 'Installing prerequisites. DNS must point to this VM; allow inbound TCP 80/443 and operator SSH in the provider firewall.'
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git python3 openssl make nano \
  restic util-linux dnsutils unattended-upgrades iproute2

# Do not remove an existing container runtime or its packages automatically.
if ! dpkg-query -W -f='${Status}' docker-ce 2>/dev/null | grep -q 'install ok installed'; then
  for package in docker.io docker-compose docker-compose-v2 docker-doc docker-buildx podman-docker containerd runc; do
    if dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -q 'install ok installed'; then
      die "Conflicting installed package: $package. Resolve with the official Docker installation guide."
    fi
  done
fi
install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat > /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
docker compose version
[[ -z $(docker ps -aq --filter label=com.docker.compose.project=marum-production) ]] ||
  die 'Existing production containers found; use the recovery/upgrade guide.'
[[ -z $(docker volume ls -q --filter label=com.docker.compose.project=marum-production) ]] ||
  die 'Existing production volumes found; do not initialize new secrets.'
[[ -z $(docker volume ls -q --filter name='^marum-production_pgdata$') ]] ||
  die 'Production database volume already exists.'
for port in 80 443 8080 8081; do
  [[ -z $(ss -H -lnt "sport = :$port") ]] || die "TCP port $port is already occupied."
done
if (( use_nginx )); then
  [[ ! -e /etc/nginx/sites-available/marum ]] || die 'Existing Marum Nginx configuration found.'
  apt-get install -y nginx certbot python3-certbot-nginx
  if (( use_cloudflare )); then
    apt-get install -y python3-certbot-dns-cloudflare
  fi
fi

# HTTPS works for public repositories. For private repos configure a root-owned,
# read-only deploy key first and set this to git@github.com:owner/repo.git.
repo_url=${MARUM_SETUP_REPO_URL:-https://github.com/andranikasd/marumbot.git}
[[ $repo_url == https://github.com/andranikasd/marumbot.git || $repo_url == git@github.com:andranikasd/marumbot.git ]] ||
  die 'Use the Marum HTTPS URL or its SSH deploy-key URL; never put tokens in clone URLs.'
if (( ! resume_before_config )); then
  # PostgreSQL's non-root entrypoint must read the bind-mounted init files.
  # Keep secrets private separately; applying umask 077 to Git makes these 0600.
  (umask 022; git clone "$repo_url" /opt/marum)
fi
cd /opt/marum
if [[ $release_ref == v* ]]; then
  commit=$(git rev-parse --verify "refs/tags/$release_ref^{commit}")
else
  commit=$(git rev-parse --verify "$release_ref^{commit}")
fi
if (( resume_before_config )); then
  [[ $(git rev-parse HEAD) == "$commit" ]] || die 'Resume release differs from the existing checkout.'
else
  (umask 022; git checkout --detach "$commit")
fi
# Also repair modes on an earlier bootstrap's private-umask checkout. These
# tracked bootstrap files contain no credentials; passwords enter via env.
chmod 0644 /opt/marum/deploy/vps/init-db.sh /opt/marum/queries/provisioning/vps-role.sql
[[ -f deploy/vps/compose.yml && -f deploy/vps/.env.example && -f migrations/00026_history_lookup_indexes.sql ]] ||
  die 'Chosen release does not contain the reviewed production preparation.'
# Build only a fresh checkout, with credentials stored elsewhere.
if find /opt/marum -path /opt/marum/.git -prune -o \
  -type f \( -name .env -o -name '.env.*' \) ! -name .env.example -print | grep -q .; then
  die 'Unexpected env file inside build context. Inspect it before building.'
fi
install -d -m 0700 /etc/marum /var/backups/marum /var/lib/marum/release-records
install -d -m 0700 /var/lib/marum/erasures
# Set numeric ownership directly: some install implementations require named
# host accounts for -o/-g. The distroless UID/GID need no host account.
python3 - <<'OWNERSHIP'
import os
os.chown('/var/lib/marum/erasures', 65532, 65532)
OWNERSHIP
export MARUM_SETUP_VERSION="sha-${commit:0:12}"
export MARUM_SETUP_TAKE_OVER=$take_over
export MARUM_SETUP_NGINX=$use_nginx
export MARUM_SETUP_CLOUDFLARE=$use_cloudflare
if (( use_nginx )); then
  cat > /etc/marum/proxy.override.yml <<'YAML'
services:
  caddy:
    profiles: [caddy]
YAML
fi

# Configuration and Telegram errors never print secret values or token URLs.
python3 - <<'PY'
import base64
import getpass
import json
import os
from pathlib import Path
import re
import secrets
import socket
import subprocess
import urllib.parse
import urllib.request

tty = open('/dev/tty')
def prompt(label):
    print(label, end='', flush=True)
    answer = tty.readline()
    if not answer:
        raise SystemExit('Interactive input ended; setup stopped.')
    return answer.strip()

env_path = Path('/etc/marum/compose.env')
if env_path.exists():
    raise SystemExit('Configuration exists; refusing to replace keys.')
domain = prompt('Public hostname (example: marum.loan): ').lower()
if len(domain) > 253 or not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', domain):
    raise SystemExit('Enter a DNS hostname only, without https:// or a path.')
if os.environ.get('MARUM_SETUP_CLOUDFLARE') != '1':
    try:
        socket.getaddrinfo(domain, 443)
    except OSError:
        raise SystemExit('Hostname does not resolve. Set DNS first.') from None
admin_domain = ''
if os.environ.get('MARUM_SETUP_NGINX') == '1':
    admin_domain = prompt('Admin hostname (example: admin.marum.loan; blank keeps SSH-only admin): ').lower()
    if admin_domain and (len(admin_domain) > 253 or not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', admin_domain)):
        raise SystemExit('Invalid admin hostname.')
    if admin_domain == domain:
        raise SystemExit('Admin needs a separate hostname.')
    if os.environ.get('MARUM_SETUP_CLOUDFLARE') == '1' and not admin_domain:
        raise SystemExit('Cloudflare setup requires both Mini App and admin hostnames.')
email = prompt('ACME certificate contact email: ')
tty.close()
if not re.fullmatch(r'[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}', email):
    raise SystemExit('Enter a valid email address.')
token = getpass.getpass('Production Telegram bot token (hidden): ').strip()
if not re.fullmatch(r'[0-9]+:[A-Za-z0-9_-]+', token):
    raise SystemExit('Invalid bot-token format.')

def telegram(method, fields=None):
    try:
        with urllib.request.urlopen(
            'https://api.telegram.org/bot' + token + '/' + method,
            data=urllib.parse.urlencode(fields or {}).encode(), timeout=20,
        ) as response:
            result = json.load(response)
        if not result.get('ok'):
            raise ValueError('rejected')
        return result['result']
    except Exception:
        raise SystemExit('Telegram check failed. Verify token/egress; no secret URL was logged.') from None

telegram('getMe')
has_webhook = bool(telegram('getWebhookInfo').get('url'))
if has_webhook:
    if os.environ['MARUM_SETUP_TAKE_OVER'] != '1':
        raise SystemExit('Bot has a webhook. Stop its old deployment and use the manual cutover guide; no webhook changed.')

admin_password = getpass.getpass('New admin password (hidden; blank disables admin): ')
if admin_domain and not admin_password:
    raise SystemExit('A public admin hostname requires an admin password.')
if admin_password:
    if len(admin_password) < 16:
        raise SystemExit('Use an admin passphrase of at least 16 characters.')
    if admin_password != getpass.getpass('Repeat admin password: '):
        raise SystemExit('Admin passwords differ.')

values = {
    'POSTGRES_PASSWORD': secrets.token_hex(32),
    'MARUM_DB_PASSWORD': secrets.token_hex(32),
    'MARUM_SERVICE_TOKEN': secrets.token_hex(32),
    'MARUM_IDENTITY_KEY': base64.b64encode(secrets.token_bytes(32)).decode(),
    'MARUM_BOT_TOKEN': token,
    'MARUM_DOMAIN': domain,
    'ACME_EMAIL': email,
    'MARUM_VERSION': os.environ['MARUM_SETUP_VERSION'],
    'MARUM_ADMIN_USER': 'admin',
}
template = Path('/opt/marum/deploy/vps/.env.example').read_text()
lines = []
for line in template.splitlines():
    key = line.partition('=')[0]
    lines.append(key + "='" + values[key] + "'" if key in values else line)
lines.append("MARUM_ADMIN_DOMAIN='" + admin_domain + "'")
fd = os.open(env_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as out:
    out.write('\n'.join(lines) + '\n')
dc = ['docker', 'compose', '--env-file', str(env_path), '-f', '/opt/marum/deploy/vps/compose.yml']
if os.environ.get('MARUM_SETUP_NGINX') == '1':
    dc += ['-f', '/etc/marum/proxy.override.yml']
subprocess.run(dc + ['config', '--quiet'], check=True)
subprocess.run(dc + ['pull', 'postgres'] + ([] if os.environ.get('MARUM_SETUP_NGINX') == '1' else ['caddy']), check=True)
subprocess.run(dc + ['build', '--pull', 'marum', 'migrate'], check=True)
if admin_password:
    result = subprocess.run(dc + ['run', '--rm', '--no-deps', '-T', 'marum', '-hash-password'],
                            input=admin_password + '\n', text=True, capture_output=True)
    if result.returncode:
        raise SystemExit('Admin password hashing failed. Configuration preserved; no app launched.')
    password_hash = result.stdout.strip()
    if not password_hash or "'" in password_hash or '\n' in password_hash:
        raise SystemExit('Unexpected hash output; no app launched.')
    env_path.write_text(env_path.read_text().replace("MARUM_ADMIN_PASSWORD_HASH=''",
                       "MARUM_ADMIN_PASSWORD_HASH='" + password_hash + "'"))
if has_webhook:
    telegram('deleteWebhook', {'drop_pending_updates': 'false'})
    if telegram('getWebhookInfo').get('url'):
        raise SystemExit('Webhook still configured; refusing to start polling.')
print('Production configuration saved privately at /etc/marum/compose.env.')
PY

if (( use_nginx )); then
  cat > /usr/local/sbin/marum-compose <<'SH'
#!/bin/sh
exec docker compose --env-file /etc/marum/compose.env \
  -f /opt/marum/deploy/vps/compose.yml -f /etc/marum/proxy.override.yml "$@"
SH
else
  cat > /usr/local/sbin/marum-compose <<'SH'
#!/bin/sh
exec docker compose --env-file /etc/marum/compose.env \
  -f /opt/marum/deploy/vps/compose.yml "$@"
SH
fi
chmod 0755 /usr/local/sbin/marum-compose
marum-compose config --quiet
marum-compose up -d --no-build --wait --wait-timeout 240
curl --fail --silent --show-error --max-time 10 http://127.0.0.1:8080/readyz \
  > /var/lib/marum/release-records/readiness.json
printf '%s\n' "$commit" > /var/lib/marum/release-records/commit.txt
docker inspect --format '{{.Image}}' "$(marum-compose ps -q marum)" \
  > /var/lib/marum/release-records/app-image.txt
if (( use_nginx )); then
  python3 - <<'PY'
import getpass
import ipaddress
import json
import os
from pathlib import Path
import re
import subprocess
import urllib.parse
import urllib.request

settings = dict(line.split('=', 1) for line in Path('/etc/marum/compose.env').read_text().splitlines()
                if '=' in line and not line.startswith('#'))
domain = settings['MARUM_DOMAIN'].strip("'")
email = settings['ACME_EMAIL'].strip("'")
admin_domain = settings.get('MARUM_ADMIN_DOMAIN', '').strip("'")
hostnames = [domain] + ([admin_domain] if admin_domain else [])
cloudflare = os.environ.get('MARUM_SETUP_CLOUDFLARE') == '1'
if cloudflare:
    with open('/dev/tty') as tty:
        print('Cloudflare zone ID (both hostnames must belong to this zone): ', end='', flush=True)
        zone_id = tty.readline().strip()
        print('VPS public IPv4: ', end='', flush=True)
        address = tty.readline().strip()
    if not re.fullmatch(r'[a-fA-F0-9]{32}', zone_id):
        raise SystemExit('Invalid zone ID.')
    try:
        if not ipaddress.IPv4Address(address).is_global:
            raise ValueError('not public')
    except ValueError:
        raise SystemExit('Enter the public IPv4 assigned to this VPS.') from None
    api_token = getpass.getpass('Cloudflare scoped API token (hidden): ').strip()
    if not re.fullmatch(r'[A-Za-z0-9_-]+', api_token):
        raise SystemExit('Invalid API token format.')

    def cf(method, path, body=None):
        request = urllib.request.Request(
            'https://api.cloudflare.com/client/v4/zones/' + zone_id + path,
            data=json.dumps(body).encode() if body is not None else None,
            headers={'Authorization': 'Bearer ' + api_token, 'Content-Type': 'application/json'},
            method=method,
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                result = json.load(response)
            if not result.get('success'):
                raise ValueError('rejected')
            return result['result']
        except Exception:
            raise SystemExit('Cloudflare API failed; check scoped token permissions/zone. No credentials were logged.') from None

    zone_name = cf('GET', '')['name'].lower()
    if any(name != zone_name and not name.endswith('.' + zone_name) for name in hostnames):
        raise SystemExit('Hostnames do not both belong to the supplied Cloudflare zone.')
    # This setting affects every hostname in a zone. Require it rather than
    # silently changing TLS for other applications sharing the user's domain.
    if cf('GET', '/settings/ssl')['value'] != 'strict':
        raise SystemExit('Set Cloudflare SSL/TLS to Full (strict), checking other zone origins first. Then resume via the Nginx guide.')
    records = []
    for name in hostnames:
        matches = cf('GET', '/dns_records?' + urllib.parse.urlencode({'name': name, 'per_page': 100}))
        address_records = [item for item in matches if item['type'] in ('A', 'AAAA', 'CNAME')]
        if address_records and (len(address_records) != 1 or address_records[0]['type'] != 'A'
                                or address_records[0]['content'] != address):
            raise SystemExit('Existing DNS points elsewhere or includes AAAA/CNAME. Perform an explicit DNS cutover first; no records changed.')
        records.append((name, address_records[0] if address_records else None))
    credentials = Path('/etc/letsencrypt/cloudflare.ini')
    credentials.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(credentials, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as out:
        out.write('dns_cloudflare_api_token = ' + api_token + '\n')
    for name, record in records:
        body = {'type': 'A', 'name': name, 'content': address, 'ttl': 1, 'proxied': True}
        if record:
            cf('PATCH', '/dns_records/' + record['id'], {'proxied': True})
        else:
            cf('POST', '/dns_records', body)

config = r'''server {
    listen 80;
    listen [::]:80;
    server_name DOMAIN_PLACEHOLDER;
    server_tokens off;
    access_log off;
    # Request URLs may contain private navigation context; use app metrics.
    error_log /dev/null;
    client_max_body_size 1m;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy no-referrer always;

    location = / { return 302 /app/; }
    location = /app { return 302 /app/; }
    location /app/ {
        # No trailing slash: preserve the /app/ prefix expected by Go.
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
        proxy_next_upstream off;
        proxy_cache off;
        proxy_buffering off;
        proxy_request_buffering off;
    }
    location / { return 404; }
}
'''
site = Path('/etc/nginx/sites-available/marum')
rendered = config.replace('DOMAIN_PLACEHOLDER', domain)
if admin_domain:
    admin_config = config.replace('DOMAIN_PLACEHOLDER', admin_domain)
    admin_config = admin_config.replace('    location = / { return 302 /app/; }\n', '')
    admin_config = admin_config.replace('    location = /app { return 302 /app/; }\n', '')
    admin_config = admin_config.replace('    location / { return 404; }\n', '')
    admin_config = admin_config.replace('location /app/ {', 'location / {')
    admin_config = admin_config.replace('http://127.0.0.1:8080;', 'http://127.0.0.1:8081;')
    admin_config = admin_config.replace('        # No trailing slash: preserve the /app/ prefix expected by Go.\n', '')
    admin_config = admin_config.replace('        proxy_cache off;', '        proxy_cache off;\n        proxy_hide_header Cache-Control;\n        add_header Cache-Control "no-store" always;\n        add_header X-Content-Type-Options nosniff always;\n        add_header Referrer-Policy no-referrer always;')
    rendered += '\n' + admin_config
with site.open('x') as out:
    out.write(rendered)
Path('/etc/nginx/sites-enabled/marum').symlink_to(site)
subprocess.run(['nginx', '-t'], check=True)
subprocess.run(['systemctl', 'enable', '--now', 'nginx'], check=True)
subprocess.run(['systemctl', 'reload', 'nginx'], check=True)
certbot = ['certbot', '--non-interactive', '--agree-tos', '--redirect', '--email', email]
if cloudflare:
    certbot += ['--authenticator', 'dns-cloudflare', '--installer', 'nginx',
                '--dns-cloudflare-credentials', '/etc/letsencrypt/cloudflare.ini',
                '--dns-cloudflare-propagation-seconds', '60']
else:
    certbot += ['--nginx']
for name in hostnames:
    certbot += ['-d', name]
subprocess.run(certbot, check=True)
subprocess.run(['nginx', '-t'], check=True)
subprocess.run(['systemctl', 'enable', '--now', 'certbot.timer'], check=True)
subprocess.run(['certbot', 'renew', '--dry-run'], check=True)
PY
fi
python3 - <<'PY'
import json
import os
from pathlib import Path
import urllib.request

ready = json.loads(Path('/var/lib/marum/release-records/readiness.json').read_text())
if not ready.get('database') or ready.get('version') != os.environ['MARUM_SETUP_VERSION']:
    raise SystemExit('Readiness/version check failed; inspect the service before opening traffic.')
domain_line = next(line for line in Path('/etc/marum/compose.env').read_text().splitlines()
                   if line.startswith('MARUM_DOMAIN='))
domain = domain_line.partition('=')[2].strip("'")
try:
    with urllib.request.urlopen('https://' + domain + '/app/', timeout=30) as response:
        if response.status != 200:
            raise ValueError('unexpected status')
except Exception:
    raise SystemExit('Containers started, but public HTTPS check failed. Check DNS/firewall and your Nginx or Caddy service. Do not rerun fresh setup.') from None
print('App launched: https://' + domain + '/app/')
admin_line = next((line for line in Path('/etc/marum/compose.env').read_text().splitlines()
                   if line.startswith('MARUM_ADMIN_DOMAIN=')), '')
admin_domain = admin_line.partition('=')[2].strip("'")
if admin_domain:
    try:
        with urllib.request.urlopen('https://' + admin_domain + '/login', timeout=30) as response:
            if response.status != 200:
                raise ValueError('unexpected status')
    except Exception:
        raise SystemExit('Admin HTTPS check failed. Check DNS, Nginx and admin bootstrap configuration.') from None
    print('Admin login: https://' + admin_domain + '/login (enroll TOTP).')
PY
marum-compose ps --all
printf '\n%s\n' \
  'Manage: sudo marum-compose ps | logs --tail=100 marum | stop marum' \
  'Admin: ssh -N -L 8081:127.0.0.1:8081 OPERATOR@VM_IP; open http://127.0.0.1:8081' \
  'Preserve /etc/marum/compose.env and its identity key in independent secure storage.' \
  'Restic is installed; offsite backup credentials, scheduling, restore drill and alerts still need configuration.' \
  'Before public release: follow docs/operations/manual-production-setup.md sections 9-10.'
