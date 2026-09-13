# Deploy the latest remote HEAD on the Ubuntu VPS

This is the repeat-deployment path for the existing installation at `/opt/marum`,
with production secrets in `/etc/marum/compose.env`. Complete
[Nginx and Cloudflare setup](nginx-cloudflare-setup.md) once before using it.
It does not rerun the bootstrap, request Cloudflare tokens or reinstall packages.

After committing and pushing the changes to the repository's **default branch**,
run on the VPS:

```bash
cd ~/marumbot
git pull --ff-only
sudo python3 deploy/vps/deploy-latest.py
```

The script fetches **`origin HEAD` from `/opt/marum`**, resolves it to a full
commit, and deploys that snapshot. It does not select the latest version tag,
use uncommitted files from `~/marumbot`, or assume the default branch is named
`main`. A push after the fetch belongs to the next deployment. There is no tag
argument. See [Git fetch](https://git-scm.com/docs/git-fetch).

The script:

1. Takes an exclusive deployment lock and refuses local content changes in
   `/opt/marum`. Earlier file-permission repairs alone do not block deployment.
2. Archives the fetched commit to an isolated build directory and builds both
   images while the old application continues serving. Images carry the full
   commit SHA; the app reports `sha-` plus its first twelve characters.
3. Stops the app, saves a PostgreSQL custom-format dump, and checks its archive
   directory. Saves the previous environment and commit privately beside it.
4. Repairs the application database role/default grants, applies migrations,
   and restores existing-table grants with the append-only ledger restrictions.
5. Checks out the commit in `/opt/marum`, atomically updates only the version
   and application/migration image settings, and recreates only the app.
6. Waits for Docker health and verifies `/readyz` reports the requested version.

Compose's [`--no-deps` and `--wait`](https://docs.docker.com/reference/cli/docker/compose/up/)
keep this app update separate from a database/proxy replacement. PostgreSQL,
its volumes, Nginx, Certbot credentials, identity keys, Telegram tokens and admin
credentials are preserved. Existing administrator passwords and TOTP enrollment
are **not reset** on deployment. PostgreSQL/proxy upgrades are separate tasks.

There is a brief outage during backup, migrations and app startup. Deploy records
and local database backups remain under `/var/lib/marum/release-records/deploy-*`
with private permissions. They have no automatic pruning; monitor free space.
These backups do not replace offsite backups or a tested restore drill. Keep the
identity key and erasure journal in the existing protected backup process.

## Verify the release

```bash
sudo marum-compose ps
curl --fail --max-time 10 http://127.0.0.1:8080/readyz
curl --fail --max-time 10 https://marum.loan/app/version
curl --fail --max-time 10 https://admin.marum.loan/login -o /dev/null
sudo nginx -t
```

Close the old Telegram Mini App, send `/start`, tap the refreshed dashboard
keyboard button, then tap the **inline button in the bot's reply**. The old
reply-keyboard web_app launch supplied no signed initData; this release replaces
it with an authenticated inline launch. The bot menu button is also supported.
Opening an old persistent keyboard without refreshing it still uses the old
Telegram button. Do not disable server authentication to work around this.

## Admin first login and diagnosis

Open `https://admin.marum.loan/login` in your browser. The bootstrap username is
`admin`; the password is the passphrase entered during installation. On **first
login**, leave the authenticator code blank, then enroll the displayed secret in
your authenticator app and verify its code. After enrollment, login requires a
fresh six-digit code. A consumed code cannot be reused; wait for the next one.

If login is refused, run this read-only diagnostic **after deploying**:

```bash
sudo python3 /opt/marum/deploy/vps/deploy-latest.py --check-admin
```

It asks for the password privately and reports only whether the configured
account is enabled, enrolled, and matches that password. It does not send a login
request, print hashes/TOTP secrets, or reset access. Share its boolean results
and the browser's exact error text when diagnosing; keep credentials private.

- Password mismatch: the entered passphrase differs from the persisted account.
  Changing `MARUM_ADMIN_PASSWORD_HASH` alone does not reset an existing identity.
- Password matches and enrolled: use a fresh authenticator code and check both
  phone and server time. `timedatectl status` should report synchronization.
- `try again later`: allow the login throttle to expire before another attempt.
- Login loops: check browser cookies, the admin hostname, and any Cloudflare
  Access policy. The origin login endpoint must bypass Cloudflare cache.
- Disabled/missing identity or an application error: inspect startup logs and
  diagnose that state before changing accounts or grants.

The prior report of admin login failure has no exact error yet; the Mini App
initData fix does not establish that admin authentication is resolved.

## Failure and recovery

Build failures leave the running app and production environment untouched. After
app stop, failures preserve the database, backup and release files and may leave
the app stopped or unhealthy. No automatic schema downgrade, volume deletion,
image pruning, password reset or guessed rollback runs.

```bash
sudo marum-compose logs --tail=100 marum
sudo marum-compose ps
```

Fix the reported cause and rerun the deploy command. If a file
`/etc/marum/compose.env.next` remains after interruption, inspect it privately and
compare it with the preserved candidate/previous environment before moving it
aside and retrying. Do not print environment files or unrestricted Compose
configuration into shared logs.

For a deliberate rollback, choose the previous release record, review that the
old binary supports the migrated schema, restore its `previous.env` to
`/etc/marum/compose.env` with mode 0600, check out `previous-commit.txt` in
`/opt/marum`, then use `sudo marum-compose up -d --no-build --no-deps --wait marum`.
Do not run migrations down. In particular, releases before encrypted admin TOTP
support are not safe rollback targets after enrollment. Consult the
[VPS rollback guide](vps.md) before restoring database data.
