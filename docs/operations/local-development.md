# Local development

Docker is the only prerequisite. There is no local Go toolchain requirement —
every target runs in a container.

## First run

```bash
cp .env.example .env             # add a Telegram bot token from @BotFather
make admin-password              # type a password; paste the hash into .env
make up                          # app, database and the observability stack
make migrate                     # apply the schema
make seed                        # demo data, so the admin interface has content
make test                        # unit tests, race detector on
```

| What | Where |
| --- | --- |
| Health, readiness, status | <http://127.0.0.1:8080/healthz> |
| Admin interface | <http://127.0.0.1:8081> |
| Grafana | <http://127.0.0.1:3000> |
| Database | `127.0.0.1:5432`, user `marum` |

The bot runs in **long-polling** mode locally, so no public URL or tunnel is
needed. Production uses webhooks; everything below the transport is the same
code.

### The admin password

`make admin-password` reads a password on stdin and prints only the hash to
stdout, so it can be piped:

```bash
make admin-password                          # prompts
MARUM_ADMIN_PASSWORD=... make admin-password # unattended, for scripts
```

The password itself never appears on a command line, where it would land in
shell history and in the process table.

## Targets

```bash
make up            # everything
make up-core       # just the app and the database
make down          # stop, keep the volume
make reset         # stop and destroy the database volume
make logs          # follow the application log

make test          # go test ./... -race
make test-short    # without the race detector
make lint          # gofumpt and golangci-lint
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

Integration tests use `testcontainers-go`, so they start their own database and
need nothing running.

## Common problems

| Symptom | Cause |
| --- | --- |
| `admin interface disabled` in the log | `MARUM_ADMIN_PASSWORD_HASH` is unset. That is the intended fail-closed behaviour. |
| `TELEMETRY DEGRADED` | The OTLP endpoint is unreachable. The service still runs; it just reports nothing. |
| Compose warns about undefined variables | A `$` in a value in `.env`. The admin hash uses `:` separators for exactly this reason. |
| Migrations fail on a dirty database | `make reset` and start again — local data is disposable by design. |
