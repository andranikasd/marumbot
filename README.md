# Marum · Մարում

A Telegram loan ledger and repayment planner with an English/Armenian Mini App,
a deterministic Go engine, and an operator admin interface. Marum records your
information and proposes repayment plans; it does not move money or connect to
bank accounts. Bank statements remain the authority for actual balances.

## Status

**v2.0.3 is deployed to development**, verified on 2026-09-03. There is no
production environment. The tagged implementation is merged into `main`.

- Mini App/API: [dev.marum.loan](https://dev.marum.loan)
- Development bot: [marum_dev_bot](https://t.me/marum_dev_bot)
- [Release v2.0.3](https://github.com/andranikasd/marumbot/releases/tag/v2.0.3)
- Database schema **22**; planning engine **`plan/5`**

[Current state](docs/current-state.md) records scope and limitations;
[release evidence](docs/design/v3/release-checklist.md) links the successful
checks and deployment. Older specifications are historical proposals, not a
claim that every described financial domain is supported.

## What is available

| Surface | Current workflows |
| --- | --- |
| Mini App | Home, Plan, Loans, Activity and More; editable loans and budgets, chosen loan icons, chart selector and legends, English/Armenian settings |
| Budget and planning | Spending limits and separate cash funding, effective policies, monthly overrides, strategy comparisons, scenarios, activation history and replay/export |
| Payments | Dated payment records, immutable corrections, statement reconciliation, known allocation coverage and actual-versus-plan reports |
| Bot | App entry points, language selection, required reminders, optional approved-plan reminders, snooze and payment links |
| Admin | Individual identities and TOTP, role/purpose controls, audited cases, policy review/publication and source-manifest replay |

Unsupported or unproven calculations return an explicit refusal or Unknown.
The correctness corpus contains both provisional and experimental coverage;
it does not certify every loan from a lender. See
[development acceptance](docs/design/v3/development-acceptance.md).

## Quick start

The Make targets use Docker; a local Go installation is not required.

```bash
cp .env.example .env
# Set a separate local Telegram token and MARUM_IDENTITY_KEY in .env.
# The identity key is a base64-encoded random 32-byte key.
make admin-password
# Put the generated hash in MARUM_ADMIN_PASSWORD_HASH if using admin.
docker compose up -d --wait postgres
make migrate
make up
make seed
```

Local endpoints: public HTTP `http://127.0.0.1:8080`, admin
`http://127.0.0.1:8081`, Grafana `http://127.0.0.1:3000`.
Admin setup includes identity/TOTP enrollment; the bootstrap password alone is
not the complete operator login flow. Local Telegram uses long polling; hosted
development uses webhooks.

See [local development](docs/operations/local-development.md) for prerequisites,
configuration, seed-data limits and troubleshooting.

## How budgeting works

The **budget is your loan spending limit**, including required payments.
**Funding is the money available to pay them.** Increasing the limit does not
create cash. Enter regular loan money and its availability date, cash already
on hand, the reserve you want to protect, and payments already made separately.
Required payments come first; extra payments require both available cash and
room in the spending limit.

A specific month's override replaces its usual limit. Expected receipts are
excluded from base-plan funding; a scenario with newly assumed cash requires
confirmation before activation. Payday does not reset spending recorded in the current
budget period. The Mini App includes a collapsed English/Armenian explanation
on Budget and Money screens. See the [budget guide](docs/product/budgeting.md).

## Architecture

Dependencies point inward: adapters → application use cases → pure core.
Contracts, bank statements and payment events are source facts. Derived balances
and reports are replayed from those facts. Plan history additionally retains
original input manifests, policy/version identities, scenario declarations and
activation records, so historical instructions can be reproduced and compared.
This is different from treating cached projections as financial truth.

```text
cmd/marum/            configuration, wiring and lifecycle
pkg/core/             money, date, model, amortisation, allocation, ledger, plan
internal/app/         use cases and consumer-owned interfaces
internal/adapter/in/  telegram, httpapi, miniapp, admin
internal/adapter/out/ postgres, telegramclient, sysclock
internal/design/      shared UI tokens
internal/i18n/        Go message catalogue
internal/corpus/      lender/reference fixture verification
queries/              named SQL embedded by queries/embed.go
migrations/           goose migrations
deploy/               containers, Cloudflare, Terraform, observability
docs/                 current guides and archived designs
```

Money uses integer minor units. Planning is per currency, without inferred FX.
Unknown lender allocation asks for a bank-reported balance. Telegram delivery
is at-least-once; a reminder can be delivered twice.

## Development

```bash
make test          # Go tests with the race detector
make test-store    # real PostgreSQL integration via Docker
make lint          # golangci-lint
make fmt           # format Go files
make migrate-check # latest migration down/up preservation check
```

`make test` skips database-dependent tests without `TEST_DATABASE_URL`.
`make test-store` is a separate integration target; CI runs the store suite with
`-race`. CI also checks Mini App behavior, bundles, migrations and architecture.
Use the [Makefile](Makefile) and [CI workflow](.github/workflows/ci.yml) for exact
commands. Golden-fixture support and known discrepancies are recorded in the
[corpus](testdata/golden/README.md).

## Releasing and deployment

A `vMAJOR.MINOR.PATCH` tag starts release validation, previous-release schema
compatibility checks, multi-platform image publication, SBOM/provenance and a
GitHub Release. A release tag does **not** deploy production.

`main` pushes can deploy development automatically. For an exact release version,
verify that `main` matches the tag's commit, then dispatch **CD · dev** from
`main` with the explicit version. Development protection rejects tag refs.
The deployment also applies the configured Terraform infrastructure before
rollout. Never reuse an existing version for changed Mini App assets.

Follow [releases](docs/operations/releases.md) and
[deployment](docs/operations/deployment.md) for checks, credentials and rollback.
The retained production workflow is manual and is not a live environment.

## Documentation and contributing

Start with the [documentation index](docs/README.md),
[engineering guide](docs/engineering-guide.md) and [agent brief](AGENTS.md).
Use Conventional Commits, subjects of at most 50 characters and a DCO sign-off
(`git commit -s`). Financial changes need fixture evidence; changes to the five
invariants require human review. Never log amounts, account identifiers or secrets.

See [Security](SECURITY.md) for private vulnerability reporting and admin access.
Local observability is provisioned by `make up`; hosted export is configurable.
The current Grafana deployment-annotation credential needs repair, although the
v2.0.3 deployment and application smoke checks passed.

## Licence

AGPL-3.0-or-later; see [LICENSE](LICENSE).
