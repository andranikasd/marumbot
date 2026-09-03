# Local development

Docker with Compose and GNU Make are required for the Make targets. There is no local Go toolchain requirement —
every target runs in a container.

## First run

```bash
cp .env.example .env             # add a separate local Telegram bot token
# Set MARUM_IDENTITY_KEY in .env to a base64-encoded random 32-byte key.
# If OpenSSL is installed: openssl rand -base64 32
make admin-password              # type a password; paste the hash into .env
docker compose up -d --wait postgres
make migrate                     # apply the schema before starting the app
make up                          # app, database and the observability stack
make seed                        # demo data, so the admin interface has content
make test                        # unit tests, race detector on
```

| What | Where |
| --- | --- |
| Liveness | <http://127.0.0.1:8080/healthz> |
| Database readiness / schema | <http://127.0.0.1:8080/readyz> |
| Queue status | <http://127.0.0.1:8080/status> |
| Mini App shell | <http://127.0.0.1:8080/app/> |
| Admin interface | <http://127.0.0.1:8081> |
| Grafana | <http://127.0.0.1:3000> |
| Database | `127.0.0.1:5432`, user `marum` |

The bot runs in **long-polling** mode locally, so no public URL or tunnel is
needed. Deployed dev uses webhooks; everything below the transport is the same
code.

### The admin password

`make admin-password` reads a password on stdin and prints only the hash to
stdout, so it can be piped:

```bash
make admin-password                          # prompts
# For automation, inject MARUM_ADMIN_PASSWORD through the environment.
# Do not type a literal password into shell history.
```

The password itself never appears on a command line, where it would land in
shell history and in the process table.

## Targets

```bash
make up            # everything
make up-core       # app/database plus their Compose dependencies
make down          # stop, keep the volume
make reset         # stop and destroy all Compose volumes
make logs          # follow the application log

make test          # go test ./... -race
make test-short    # without the race detector
make test-store    # migrated Compose Postgres; store tests, without -race
make lint          # golangci-lint (make fmt applies gofumpt)
make vet           # go vet
make fmt           # format in place

make migrate         # apply pending migrations
make migrate-down    # roll back one
make migrate-status  # what is applied
make migrate-check   # up, down, up
make seed            # demo data
make shell           # psql

make load          # k6 load profile against the local stack
make grafana       # print the Grafana URL
```

## The demo data

`make seed` inserts three loans chosen to exercise **reliability grading**
rather than to look tidy:

| Loan | State | Why |
| --- | --- | --- |
| Consumer loan | `confirmed` | Bank-confirmed anchor, clean ledger |
| Card balance | `estimated` | The borrower typed a balance but never confirmed it |
| Car loan (USD) | `unsupported` | Carries penalties and overdue principal |

The third is the important one: reminders continue, projections stop.

## Running tests against real Postgres

Run `make test-store`: it starts/waits for Compose `postgres`, applies goose
migrations, and runs `./internal/adapter/out/postgres/` with `TEST_DATABASE_URL`
on `marum_default`. It uses the local `marum` database, so use disposable local
data. There is no testcontainers setup. `make test` uses the race detector but
skips these store tests because it does not supply `TEST_DATABASE_URL`; CI's
separate store job supplies the URL and runs with `-race`.

`make up-core` also starts `otel-collector` and its dependencies (`tempo`,
`loki`, `prometheus`) because `marum` depends on the collector. It does not
start Grafana or Pyroscope. Compose overrides the app's database and OTLP URLs
and supplies a local Pyroscope URL. An empty OTLP value in `.env` therefore
does not disable telemetry in this Compose setup.

The local Mini App shell is viewable in a browser, but authenticated API calls
need Telegram-signed initData. Telegram launch buttons need a reachable HTTPS
`MARUM_MINIAPP_URL`; loopback is only useful for local shell inspection.
`MARUM_TICK_INTERVAL` accepts seconds (`60`) or a duration (`60s`).

## Common problems

| Symptom | Cause |
| --- | --- |
| `admin interface disabled` in the log | `MARUM_ADMIN_PASSWORD_HASH` is unset. That is the intended fail-closed behaviour. |
| `TELEMETRY DEGRADED` | Telemetry initialization failed. The service still runs; inspect the accompanying error. Export failures after startup may appear differently. |
| Compose warns about undefined variables | A `$` in a value in `.env`. The admin hash uses `:` separators for exactly this reason. |
| Migrations fail on a dirty database | Inspect the migration error first. Only reset disposable local data; `make reset` deletes every Compose volume, including observability history. |
