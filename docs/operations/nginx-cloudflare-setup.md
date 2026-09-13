# Ubuntu + Nginx + Cloudflare + Certbot

Use [bootstrap-ubuntu.sh](../../deploy/vps/bootstrap-ubuntu.sh) for a fresh
8-vCPU / 24-GB Ubuntu 22.04, 24.04 or 26.04 VPS and a new production database.
This extends the [manual guide](manual-production-setup.md): host Nginx replaces
Caddy, and an HTTPS admin hostname replaces SSH-only admin access.

```text
Telegram Mini App → Cloudflare → HTTPS Nginx → 127.0.0.1:8080 → Marum
Admin browser    → Cloudflare → HTTPS Nginx → 127.0.0.1:8081 → Admin
Marum → private Docker network → PostgreSQL
Marum → outbound Telegram polling
```

Both hostnames get a Let's Encrypt certificate installed on the VPS by Certbot.
Cloudflare also presents its edge certificate to visitors. Full (strict) TLS
validates the certificate between Cloudflare and Nginx. No Worker, Tunnel,
Cloudflare Container, Redis or Valkey is required.

## Before running

Choose two distinct hostnames in the same active Cloudflare zone, for example
`marum.loan` and `admin.marum.loan`. These are examples: the installer prompts
for the real names. Have your VPS public IPv4, production bot token, an admin
passphrase, and a reviewed **pushed** tag/full commit containing schema 26 ready.
Do not use the old `v1.0.0` tag for the current production implementation.

In Cloudflare:

1. Confirm the domain's authoritative nameservers point to Cloudflare and the
   zone is active. Copy the **Zone ID** from the zone overview.
2. Set **SSL/TLS → Overview → Full (strict)**. This is a zone-wide setting;
   verify other origins in the zone have valid certificates before changing it.
   The installer checks it and refuses to silently change TLS for other sites.
3. Create a custom API token restricted to **this zone only**, with
   **Zone / DNS / Edit**, **Zone / Zone / Read**, and
   **Zone / Zone Settings / Read**. The installer reads the zone/settings,
   creates the two A records, and Certbot uses DNS Edit for certificate renewal.
   Do not use the Global API Key. If token expiry or IP restrictions are set,
   they must permit renewal from this VPS.
4. Remove or narrow any old Worker routes/custom domains covering these two
   hostnames. A Worker matching `marum.loan/*` can intercept traffic before Nginx.
   Check redirect/origin rules for old deployment targets too.
5. Create a **Cache Rule** bypassing cache for both hostnames initially:

   ```text
   (http.host eq "marum.loan") or (http.host eq "admin.marum.loan")
   ```

   Set cache eligibility to **Bypass cache**; ensure other rules do not override
   it. Browser caching of versioned static assets remains available. Never cache
   admin pages, authenticated APIs, or the `/app/version` response. Disable Rocket
   Loader for the Mini App and avoid interactive challenges that prevent its API
   calls in Telegram's webview. Test real Telegram behavior with your WAF rules.

The script creates these records with proxying enabled (orange cloud):

| Type | Name | Content | Proxy |
| --- | --- | --- | --- |
| A | Your Mini App hostname | VPS public IPv4 | Proxied |
| A | Your admin hostname | VPS public IPv4 | Proxied |

It reuses an existing A record only if it already points to that IPv4. An old
target, CNAME, or AAAA record stops setup rather than silently moving an existing
application. Plan that DNS cutover yourself first. Unrelated TXT/MX records are
left alone. A failed API request can leave one record created: inspect DNS before
resuming, rather than assuming the two writes were atomic.

Allow inbound TCP 80/443 and operator-only SSH in the provider firewall. Keep
5432, 8080 and 8081 inaccessible externally. Outbound HTTPS and DNS are required.
DNS-01 validation works with proxying enabled and does not need public challenge
files; the [Certbot Cloudflare plugin](https://certbot-dns-cloudflare.readthedocs.io/en/stable/)
creates and removes temporary TXT records. If later limiting origin HTTP/S to
Cloudflare IP ranges, maintain those ranges and retain an operator diagnostic
path. This installer does not change provider firewall rules or SSH settings.

## Run the installer

Copy the script from your **workstation repository** to the VPS:

```bash
scp deploy/vps/bootstrap-ubuntu.sh ubuntu@YOUR_VPS:~/bootstrap-ubuntu.sh
ssh ubuntu@YOUR_VPS
sudo bash ~/bootstrap-ubuntu.sh YOUR_PUSHED_TAG_OR_FULL_COMMIT --cloudflare
```

`--cloudflare` also enables Nginx. For Nginx without Cloudflare DNS automation,
use `--nginx` instead and create DNS records yourself; that path uses HTTP-01.
Without either flag, the original Caddy/SSH-admin deployment is used.

For a private GitHub repository, first install a read-only deploy key for the
root Git client and verify GitHub's SSH host key. Then run:

```bash
sudo env MARUM_SETUP_REPO_URL=git@github.com:andranikasd/marumbot.git \
  bash ~/bootstrap-ubuntu.sh YOUR_PUSHED_TAG_OR_FULL_COMMIT --cloudflare
```

No token belongs in the clone URL. The script prompts interactively for:

- Mini App hostname, admin hostname, and certificate contact email.
- Production Telegram token and an admin passphrase of at least 16 characters.
- Cloudflare zone ID, VPS IPv4 and scoped API token.

Secrets use hidden prompts. Database passwords, service token and identity key
are generated independently in `/etc/marum/compose.env` with mode `0600`, outside
the build context. The admin hash is generated by the built application and saved
without showing the password. The build stamp is `sha-` plus the first 12 commit
characters, making the running binary traceable to the selected source.

For a reused bot, stop the old deployment/scheduler first. Add `--take-over-bot`
to allow removal of its webhook after the build succeeds. Pending updates are
preserved. Without this flag a configured webhook stops setup. No flag can prove
another poller is stopped: ensure that yourself before using the token.

The script refuses an existing `/opt/marum`, `/etc/marum`, journal, production
containers or database volume. It is a first-install script, not an updater or
restore tool. It preserves partial state on failure rather than deleting data.

## What gets installed

| Location/service | Purpose |
| --- | --- |
| `/opt/marum` | Detached checkout of chosen release |
| `/etc/marum/compose.env` | Private production configuration and identity key |
| `/etc/marum/proxy.override.yml` | Puts Caddy behind an inactive profile |
| `/usr/local/sbin/marum-compose` | Explicit production Compose wrapper including override |
| `/var/lib/marum/erasures` | Private independent erasure journal |
| `/etc/nginx/sites-available/marum` | Mini App and admin virtual hosts |
| `/etc/nginx/sites-enabled/marum` | Enabled site link |
| `/etc/letsencrypt/cloudflare.ini` | Root-only DNS token used again during renewal |
| `/etc/letsencrypt/live/<certificate-name>/` | Certificate and private key maintained by Certbot |
| `certbot.timer` | Automated certificate renewal |
| `/var/lib/marum/release-records/` | Commit, application image ID and readiness evidence |

Docker runs PostgreSQL, a one-shot migrator and Marum. Nginx/Certbot run on Ubuntu.
Always use `sudo marum-compose ...` for this setup. Starting the base Compose file
alone would try to start Caddy and collide with Nginx on 80/443. The override must
remain present across upgrades. Do not explicitly start the `caddy` profile.

The Mini App virtual host permits `/app/`, redirects `/` and `/app` to `/app/`,
and returns 404 for everything else. The admin virtual host proxies to the admin
listener on 8081. It retains the application's password/TOTP, secure cookies,
role checks and auditing. Complete enrollment immediately on first login.

The essential routing is:

```nginx
# Inside the Mini App server block:
location /app/ {
    proxy_pass http://127.0.0.1:8080;
}
location / { return 404; }

# Inside the separate admin server block:
location / {
    proxy_pass http://127.0.0.1:8081;
}
```

The installer supplies the full configuration, timeouts, forwarded host/protocol
headers, body limit, and disabled proxy caching/retries/buffering. Do not add a
trailing slash to `proxy_pass` for the Mini App: [Nginx would replace the location
prefix](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass), but
Go expects `/app/`. Request access/error logs are disabled for these virtual hosts
because URLs can contain private navigation context; use app metrics/readiness
and `nginx -t` for diagnostics. Admin responses carry `Cache-Control: no-store`.

## Certificate issuance and renewal

The installer runs the equivalent of:

```bash
sudo certbot --non-interactive --agree-tos --redirect \
  --authenticator dns-cloudflare --installer nginx \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  --dns-cloudflare-propagation-seconds 60 \
  --email YOUR_EMAIL -d YOUR_MINIAPP_HOST -d YOUR_ADMIN_HOST
sudo systemctl enable --now certbot.timer
sudo certbot renew --dry-run
```

It requests a certificate covering both names and lets Certbot install HTTPS and
HTTP redirects in Nginx. This is a publicly trusted origin certificate, not a
Cloudflare Origin CA certificate. Keep the DNS token available for renewal;
Certbot stores its file path, not the token contents in its renewal configuration.

Inspect status without printing private keys or the token:

```bash
sudo certbot certificates
sudo systemctl list-timers certbot.timer
sudo nginx -t
sudo systemctl status nginx --no-pager
```

If rotating the token, securely update `/etc/letsencrypt/cloudflare.ini`, retain
mode `0600`, and run another dry renewal. If you set CAA records, they must permit
Let's Encrypt. Check the zone's edge certificate covers both names too; deeper
subdomains may require additional Cloudflare edge certificate coverage.

## Verify both routes

Replace the hostnames in these **workstation** commands:

```bash
curl --fail https://marum.loan/app/version
curl -sS -o /dev/null -w '%{http_code}\n' https://marum.loan/app/          # 200
curl -sS -o /dev/null -w '%{http_code}\n' https://marum.loan/app/api/loans # 401
curl -sS -o /dev/null -w '%{http_code}\n' https://marum.loan/readyz        # 404
curl -sS -o /dev/null -w '%{http_code}\n' https://marum.loan/status        # 404
curl -sS -o /dev/null -w '%{http_code}\n' https://admin.marum.loan/login   # 200
```

On the **server**:

```bash
sudo marum-compose ps --all
curl --fail http://127.0.0.1:8080/readyz
sudo nginx -t
sudo certbot renew --dry-run
```

The public `/app/version` must match local readiness. A mismatch can indicate a
stale Worker route or cached old deployment. Open the production Mini App in
Telegram and admin in a browser; complete password/TOTP and fixture-backed smoke
checks. No database or raw application port should be public.

To test the origin certificate independently of Cloudflare, run from the VPS
with the real hostname (loopback deliberately bypasses Cloudflare DNS):

```bash
curl --fail --resolve marum.loan:443:127.0.0.1 https://marum.loan/app/version
curl --fail --resolve admin.marum.loan:443:127.0.0.1 https://admin.marum.loan/login -o /dev/null
```

Do not use `-k`: certificate validation is part of the test. Cloudflare
[Full (strict)](https://developers.cloudflare.com/ssl/origin-configuration/ssl-modes/full-strict/)
requires an unexpired certificate matching the requested hostname.

## If setup stops partway

If setup stopped at `install: invalid user: '65532'` before creating
`/etc/marum/compose.env`, copy the corrected installer to the host, stop the
unused default Nginx service with `sudo systemctl stop nginx`, and run:

```bash
sudo bash ~/bootstrap-ubuntu.sh YOUR_SAME_TAG_OR_FULL_COMMIT --cloudflare --resume-before-config
```

This narrow resume mode requires the same clean checkout, no config, no
production containers/volumes and an empty journal. Numeric ownership now uses
Python `os.chown`, without requiring a host user named `65532`. Do not create a
host account or delete the checkout to work around this error.

Do not rerun the first-install script or remove `/etc/marum` to get past its
guard. Preserve generated keys and any database volume. Inspect the last error.
If the wrapper is not yet installed, use the explicit Compose command from the
manual guide, adding `-f /etc/marum/proxy.override.yml` for this deployment.

- Git/private-repo errors: fix read-only deploy-key access; preserve the checkout.
- Build errors: resolve the error, then build the existing checkout/config.
- Cloudflare token/zone/TLS errors: correct permissions and Full (strict); inspect
  both records before completing them manually in DNS. Never replace a different
  target without an intentional cutover.
- Certificate errors after Nginx config exists: resolve the DNS token, CAA or API
  issue, rerun the certificate command above with the existing credentials, then
  `nginx -t`, reload Nginx, and run a dry renewal.
- If Nginx config was not yet created, complete the virtual hosts using the
  installer template before rerunning Certbot; don't issue certificates against
  an unrelated default virtual host.
- Cloudflare 526: check origin certificate/hostname using `--resolve` above.
- Cloudflare 521/522: check Nginx, VM/provider firewall, port 443 and origin IP.
- A challenge page in Telegram: review WAF/Bot rules for the Mini App/API.
- Healthy containers but public version differs: remove old Worker/origin routes
  and cached responses for these exact hostnames.

The script installs restic but does not configure offsite storage, backup timers
or alert receivers without their credentials. Complete sections 9–10 of the
[manual production guide](manual-production-setup.md#9-enable-independent-encrypted-backups-and-rehearse-recovery)
before releasing to borrowers. Preserve the identity key and authoritative journal
independently. Switching proxies does not change those recovery requirements.
