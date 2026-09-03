# Engineering guide

Current implementation reference for v2.0.4. Read [current state](current-state.md)
for release scope and [AGENTS.md](../AGENTS.md) for the governing invariants.
Historical design documents describe intent; they are not evidence of runtime
behavior. Some conventions require review as well as automated checks.

## 1. Repository structure

```text
cmd/marum/                  wiring, configuration, lifecycle
pkg/core/money/             checked integer amounts, rates, rounding, accrual
pkg/core/date/              date-only arithmetic and anchored schedules
pkg/core/model/             contracts, snapshots, events and state
pkg/core/amortisation/      dated schedules and payment solving
pkg/core/allocation/        lender-policy allocation
pkg/core/ledger/            replay
pkg/core/plan/              budgets, funding, strategies and plan assembly
internal/app/               use cases and consumer-owned port interfaces
internal/adapter/in/        telegram, httpapi, miniapp, admin
internal/adapter/out/       postgres, telegramclient, sysclock
internal/design/            shared stylesheet tokens
internal/i18n/              Go English/Armenian catalogue
internal/obs/               OpenTelemetry, redaction and metrics
internal/config/            configuration validation
internal/corpus/            filesystem-backed reference fixture tests
queries/                    named SQL embedded by queries/embed.go
migrations/                 external goose SQL applied by the migration runner
deploy/                     containers, Cloudflare, Terraform, observability
```

Pure financial logic belongs in `pkg/core`; orchestration and transaction
boundaries belong in `internal/app`; protocol translation belongs in adapters.
There are no `utils`, `helpers`, `common` or `shared` packages.

## 2. The dependency rule

Dependencies point inward: adapters → app → core. Consumer interfaces sit next
to their use cases: for example `loans.go`, `worker.go`, `loan_commands.go`,
`payment_reconciliation.go` and `plan_history.go` in `internal/app`.
There is no central `ports.go` or `apptest` package.

[depguard configuration](../.golangci.yml) restricts core I/O imports, app-to-adapter
imports and adapter-to-inbound-adapter imports. Review must still assess purity
and coupling beyond those patterns. CI's standalone engine job imports the money
package and runs accrual; it is not a standalone proof of every planner path.

## 3. Naming

Use domain names, small capability interfaces and consistent receivers.
Amounts represented as raw integers carry their unit (`principalMinor`);
dates distinguish `valueDate`, `postedAt` and `asOf`. Test names state the case.
Avoid implementation names in interfaces and vague Manager/Helper/Util types.

## 4. Domain types

Money is `money.Amount`, with private integer minor units and a currency.
`Add` and `Sub` return errors for overflow, but panic on mixed currencies through
`mustMatch`: callers must enforce currency consistency before arithmetic. Rates
use `money.Rate`.

Some model structs have exported fields. Validate them at the relevant domain
boundary rather than assuming constructors make invalid states impossible.
Panic is reserved for programming errors, not user-input rejection.
Unsupported allocation remains `unknown/v0`; use a bank-reported balance rather
than deriving an unvalidated result. See [money and dates](architecture/04-money-and-dates.md).

## 5. Errors

Wrap errors with `%w` when adding useful context; inspect them with `errors.Is`
and `errors.As`. Use lowercase messages without trailing punctuation. Error
strings must not include balances, amounts, identifiers, tokens or raw input:
they may reach logs. Return localized user-facing failures and a correlation ID
where available, rather than exposing diagnostics.

## 6. Context and concurrency

Pass `context.Context` first for I/O and propagate cancellation. Every goroutine
must have an owner able to stop and wait for it. Keep lock scopes bounded.
Webhook handlers persist durable inbox work; workers own longer processing.
Do not make network calls inside database transactions. No sleeping tests:
inject clocks or synchronize explicitly.

## 7. Interfaces and wiring

Use small interfaces declared by consumers and explicit constructor injection.
`cmd/marum` composes concrete adapters and application services. Application
transaction ports define what is atomic; PostgreSQL implements those ports.
Avoid service locators and hidden global application dependencies.

## 8. Database and SQL

- SQL statements live in `queries/*.sql`. `queries/embed.go` embeds them and
  resolves `-- name:` declarations through `Get`/`Lookup`. They are not currently
  compiled by sqlc; that is a historical tooling proposal.
- Financial facts are append-only. Corrections append reversals/replacements,
  and statement coverage must remain consistent. Do not repair facts by editing
  history.
- Aggregate writes use optimistic versions and durable command identities.
  Resolve a conflict from current facts; do not blindly repeat a stale write.
- The application owns transactions; adapters implement the database operations.
  Resolve required nontransactional context before opening a transaction and
  avoid acquiring another pool connection inside it.
- Migrations run through goose separately from the app image; they are not
  embedded in the application binary. They are expand-only and must remain compatible with the previous
  released binary. Money columns use `bigint`; exact rate representations and
  constraints are defined in the migrations, not a universal guessed SQL type.
- Plan history stores original manifests, declarations, version/source identities
  and activations. Derived reports are recomputed. This existing design does not
  authorize new persisted recomputable data without the review required by AGENTS.

See [database](architecture/05-database.md) and [ledger replay](architecture/03-ledger-replay.md).

## 9. Logging and telemetry

Use structured `slog`. Never log request bodies, amounts, chat/account/loan IDs
or secrets. `internal/obs` redacts typed amounts and sensitive attribute keys;
redaction is a second layer, not permission to emit arbitrary raw strings.
Metric labels must have bounded cardinality independent of account growth.
An empty OTLP endpoint leaves stdout logging available; it does not silence the
service. See [observability](architecture/08-observability.md).

## 10. Testing

| Check | Current mechanism |
| --- | --- |
| Unit, core, corpus and journeys | `make test`: Go tests with `-race`; hand-written fakes alongside tests |
| Real PostgreSQL | `TEST_DATABASE_URL`; `make test-store` provisions Docker/PostgreSQL and migrations |
| Store race checking | CI runs the PostgreSQL suite with `-race`; the local `test-store` target currently omits it |
| Frontend behavior | Node suites in the Mini App; CI also checks bundle parsing and rollout helpers |
| Migration compatibility | Latest down/up preservation checks and release previous-binary/schema compatibility |
| Formatting and lint | `make fmt`, `make lint`; CI checks formatting rather than silently changing it |

`make test` skips database-dependent tests when no test database is configured.
`make test-short` is a convenience run without `-race`, not release evidence.
The desired race gate remains required for release validation. There is no
`testcontainers-go` harness or blanket network-disabled CI environment.

Add a regression at the responsible layer. A reported financial discrepancy
needs a source fixture before the fix. Preserve fixture values and provenance;
known mismatches are recorded by the corpus ratchet, not rounded away.
See [corpus coverage](../testdata/golden/README.md) and [CI](../.github/workflows/ci.yml).
Documentation-only changes need source/link checks, not an unrelated engine test run.

## 11. The five invariants

The five invariants in [AGENTS.md](../AGENTS.md) remain unchanged and require
human review to change. Current enforcement is narrower than the old guide
claimed:

| Invariant | Evidence and enforcement |
| --- | --- |
| Integer money | `money.Amount`, arithmetic/corpus tests and review; forbidigo bans `strconv.ParseFloat`, not the `float64` type everywhere |
| Injected time | forbidigo restrictions on `time.Now`/`time.Since`, with the sysclock exception |
| Pure deterministic core | depguard restrictions, standalone money import check, tests and review |
| Append-only facts and versioned cache | Database guards, named query review, replay and integration tests |
| No sensitive telemetry | Redacting handler, telemetry tests and review; lint is not complete data-flow analysis |

**Policy reference needing separate review:** invariant 1 still names
`pkg/core/rates` as the sole float exception, but that package does not exist.
Current rates live in `pkg/core/money`; the production core paths inspected for
this audit contain no `float64` arithmetic, while an accrual test uses a float
reference calculation. The literal package exception and comprehensive lint
claim cannot be treated as an implemented guarantee. This documentation update
preserves the invariant and does not authorize changing money representation or
adding new float paths.

## 12. Tooling

The [Makefile](../Makefile) pins container tooling; [go.mod](../go.mod) declares
the module's language floor and libraries. Actual tooling includes gofumpt,
golangci-lint, goose, pgx, Node for frontend tests and the Cloudflare build tools.
The Go catalogue lives in `internal/i18n/catalogue.go`; Mini App strings live in
`web/js/i18n.js` and screen `addStrings` declarations. There are no `en.toml`,
`hy.toml` or `ru.toml` catalogues, and no sqlc/go-i18n generation step.

## 13. Git

Use short-lived branches from `main` and respect explicitly requested branch
names. Conventional Commit subjects are at most 50 characters; sign off with
`git commit -s`. Keep commits cohesive and never rewrite published main history.
PRs explain the resulting behavior and relevant verification. A core change
names its fixture coverage. Respect the actual repository/environment protection
settings; this guide does not assert a particular GitHub merge configuration.

## 14. Definition of done

- Relevant behavior has regression evidence; financial numbers have fixtures.
- Required lint, race, frontend and database checks pass for the affected scope.
- Migrations preserve existing data and previous-binary compatibility.
- No unreviewed persisted derived data or new unbounded metric labels.
- User-facing strings are present in English and Armenian in their real catalogues.
- Errors/telemetry do not expose sensitive data; delivery remains at-least-once.
- A release uses a new immutable asset version and verifies the full development
  deployment, not only frontend assets. See [release steps](operations/releases.md).

## 15. Working with AI coding agents

Read AGENTS and the current guides before modifying behavior. Keep its compact
layout and commands aligned with the repository. Do not use a documentation
correction to weaken an invariant or expand unvalidated lender behavior.

## 16. Deliberately not rules

Line length beyond the formatter, named returns, local declaration style and
small helper placement are author choices. Prefer clear names and readable
scope over new style rules without a concrete maintenance benefit.
