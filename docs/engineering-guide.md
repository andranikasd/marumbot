# Engineering guide

How Marum is written and how the repository is laid out.

Companion to [`design/Marum-MVP-Design.pdf`](design/Marum-MVP-Design.pdf), which
says *what* is built. This says *how*.

**One rule about the rules:** every convention below is enforced by a linter, a
CI check, or a test — or it is deleted. A style rule that depends on a reviewer
remembering it is not a rule, it is a preference, and preferences belong in
§17 where they cost nobody an argument.

---

## 1. Repository structure

```
marumbot/
├── cmd/
│   └── marum/
│       └── main.go             flags, config, wiring, signals, shutdown. No logic.
├── pkg/                        public API — importable by anyone, AGPL applies
│   └── core/                   the calculation engine: pure, deterministic, no I/O
│       ├── money/              Amount, Currency, RoundingPolicy, arithmetic
│       ├── model/              Contract, Snapshot, Event, LoanState, Plan
│       ├── amort/              dated schedules, accrual, the payment solver
│       ├── alloc/              payment allocation policies (§ design 5)
│       ├── ledger/             replay(contract, snapshot, events) -> LoanState
│       ├── rates/              day-count conventions; the only float64 in the tree
│       └── plan/               strategies, budgets, plan assembly
├── internal/                   everything that touches the outside world
│   ├── app/                    use cases; owns the port interfaces
│   │   ├── loanservice.go
│   │   ├── paymentservice.go
│   │   ├── planservice.go
│   │   ├── notifyservice.go
│   │   ├── userservice.go
│   │   ├── reconcile.go        nightly ledger replay job
│   │   └── ports.go            Repository, Sender, Clock, BlobStore
│   ├── adapter/
│   │   ├── in/
│   │   │   ├── telegram/       update router, conversation machine, keyboards
│   │   │   └── httpapi/        health, readiness, internal endpoints
│   │   └── out/
│   │       ├── postgres/       pgx pool, sqlc output, tx helper
│   │       ├── telegramclient/ the single rate-limited sender
│   │       ├── blob/           object storage for dumps and exports
│   │       └── sysclock/       the only place time.Now() is allowed
│   ├── obs/                    OTel setup, slog handler, redaction, metrics
│   ├── config/                 env parsing and validation
│   └── corpus/                 replays testdata/golden; here because it reads files
├── migrations/                 goose SQL, expand-only, embedded in the binary
├── queries/                    .sql consumed by sqlc; never hand-written Go SQL
├── locales/                    en.toml, hy.toml, ru.toml
├── testdata/
│   └── golden/                 the correctness corpus: schedules real lenders issued
├── deploy/                     Dockerfile, docker-compose.yml, self-hosting docs
├── docs/                       design documents, diagrams, this file
├── .golangci.yml               the enforcement backbone
└── AGENTS.md                   the short version, for AI coding agents
```

### What goes where

| Question | Answer |
| --- | --- |
| Pure arithmetic on loans? | `pkg/core` |
| Needs a database, a clock, or a network? | `internal` |
| Orchestrates several core calls in one transaction? | `internal/app` |
| Translates a protocol (Telegram, HTTP, SQL)? | `internal/adapter` |
| A business rule? | **Never** in an adapter. If a rule lives in an adapter it cannot be tested without a socket. |
| Genuinely generic, no domain meaning? | Standard library, or a dependency. Not a `utils` package. |

There is no `pkg/utils`, no `internal/helpers`, no `common`, and no `shared`.
Those names are where cohesion goes to die. If something has no better home, it
usually means the thing it belongs to has not been named yet.

---

## 2. The dependency rule

Dependencies point inward. `pkg/core` is the centre and knows nothing about the
rest of the world.

```
adapter/in ──▶ app ──▶ pkg/core
                 ▲
adapter/out ─────┘   (implements interfaces that app declares)
```

Four boundaries, all machine-checked by `depguard`:

1. **`pkg/core` may import only the standard library**, and not the parts of it
   that touch time, randomness, the filesystem, or the network.
2. **`pkg/core` must never import `internal/`.** The engine is publishable on
   its own; the day it imports the application, it stops being.
3. **`internal/app` must never import an adapter.** It declares interfaces;
   adapters satisfy them.
4. **Adapters must never import each other.** Postgres does not know Telegram
   exists.

A test binary that imports only `pkg/core` and computes a full plan is part of
CI. If someone adds an I/O dependency to the engine, that binary stops
compiling — which is a better error message than any lint rule.

---

## 3. Naming

| Rule | Example |
| --- | --- |
| No stutter | `loan.Service`, never `loan.LoanService` |
| Packages are lowercase nouns, no underscores, no plurals | `amort`, `postgres`, `money` |
| Interfaces name a capability, not an implementation | `Sender`, not `TelegramSenderInterface` |
| Receivers are one or two letters, consistent per type | `func (a Amount) Add(...)`, always `a` |
| Money variables carry the unit | `principalMinor`, `amountMinor` — never a bare `amount` for an integer |
| Dates say what kind | `valueDate`, `postedAt`, `asOf` — never a bare `date` |
| Constructors validate | `NewContract(...) (Contract, error)`, not a naked struct literal |
| Test names describe the case | `TestSolvePayment_FebruaryLeapYear`, not `TestSolvePayment2` |
| No `Manager`, `Helper`, `Util`, `Handler` as a type suffix | ...unless it genuinely handles HTTP |

Abbreviations stay whole in identifiers except the Go-standard ones (`id`,
`url`, `http`, `db`). No `cfg`, `req`, `res`, `svc` in exported names. Local
variables may abbreviate when the scope is under ten lines.

---

## 4. Domain types

The domain is modelled with types that make wrong states unrepresentable. This
is where the project spends its complexity budget.

```go
// Good: the unit and the currency are in the type, arithmetic is checked.
type Amount struct {
    minor int64          // unexported — no arithmetic outside this package
    cur   Currency
}

// Bad: nothing stops this being dollars, or a float, or a rate.
type Amount int64
```

Rules:

- **Never `float64` for money.** Anywhere. The only package permitted to use
  `float64` at all is `pkg/core/rates`, which produces rates, not amounts, and
  rounds before returning.
- **Never a naked struct literal for a domain type outside its own package.**
  Construction goes through a constructor that validates. A `Contract` that
  fails validation must not be constructible.
- **Enumerations are string types with a validated set**, not `int` constants,
  because they are persisted and read by humans in `jsonb` and in logs.
- **Panic is for programmer error only.** Adding AMD to USD panics — it is a
  bug, not a runtime condition. Everything a user can cause returns an error.
- **Zero values must be safe or impossible.** A zero `Amount` is zero AMD-less
  money and is only valid as an accumulator seed; a zero `Contract` is rejected
  by every function that takes one.

---

## 5. Errors

```go
// Sentinel for a condition callers branch on.
var ErrPolicyUnknown = errors.New("allocation policy not established for this lender")

// Typed for a condition callers need detail from.
type NegativeAmortisationError struct {
    LoanID ID
    Period int
}

func (e NegativeAmortisationError) Error() string {
    return fmt.Sprintf("payment does not cover interest and fees: loan %s period %d",
        e.LoanID, e.Period)
}
```

- Wrap with `%w` when adding context, and add context that the caller does not
  already have. `fmt.Errorf("loading loan: %w", err)` is useful;
  `fmt.Errorf("error: %w", err)` is noise.
- Compare with `errors.Is` and `errors.As`. Never with string matching.
- **Error strings are lowercase, no trailing punctuation**, and never contain a
  balance, an amount, a chat ID, or a token. They end up in logs.
- A function that returns only one error kind and no other value should probably
  return a `bool` instead. `func (p Policy) Allows(d Date) bool`.
- Errors shown to users are generic and localised; the diagnostic detail stays
  in the trace, addressed by a request correlation ID the user can quote.

---

## 6. Context and concurrency

- `ctx context.Context` is the first parameter of any function that does I/O.
  It is never stored in a struct.
- No values in `context` except request-scoped correlation IDs, set in exactly
  one middleware.
- **No bare `go func()`.** Every goroutine has an owner that can wait for it and
  a way to stop it — `errgroup.Group`, or a struct with `Start`/`Stop`. A
  goroutine started inside an HTTP handler outlives the request and is a leak.
- Long work is enqueued, never awaited inside the Telegram webhook. The webhook
  answers within two seconds.
- Shared mutable state is either owned by one goroutine or protected by a mutex
  held for a bounded, obvious span. If a mutex is held across an I/O call,
  that is a bug in the design, not in the locking.

---

## 7. Interfaces and wiring

Interfaces are declared by the **consumer**, in `internal/app/ports.go`, and are
as small as the consumer's need:

```go
// app owns this. postgres implements it. app does not import postgres.
type Repository interface {
    LoanState(ctx context.Context, id ID) (core.LoanState, error)
    AppendEvent(ctx context.Context, e core.Event, expectVersion int64) error
    ClaimNotifications(ctx context.Context, owner string, n int) ([]Notification, error)
}
```

Composition happens once, in `main`, with explicit constructor injection. No DI
framework, no service locator, no package-level singletons except the OTel
providers. If wiring in `main` becomes unpleasant to read, that is information
about the design, and the answer is fewer dependencies rather than a container
to hide them in.

Accept interfaces, return concrete types.

---

## 8. Database and SQL

- **All SQL lives in `queries/*.sql`** and is compiled by `sqlc`. No SQL strings
  in Go, no query builders, no ORM. A hand-built query is a review blocker.
- **`loan_events` and `billing_events` are append-only.** No `UPDATE`, no
  `DELETE`, ever. A mistake is a `reversal` row. This is enforced by a database
  `RULE`-level guard in migrations *and* by a test that asserts the generated
  code contains no update or delete for those tables.
- **Every write to `loan_state` carries the optimistic lock.** Zero rows
  affected means someone moved first: reload, recompute, retry.
- **Transactions are opened in `internal/app`, never in an adapter**, because
  the use case knows what must be atomic and the adapter does not.
- **No network call inside a transaction.** The notification claim commits
  before Telegram is contacted; see design §7.1.
- Migrations are expand-only and embedded. A migration that breaks the previous
  binary is split across two releases.
- `timestamptz` for instants, `date` for business dates, `bigint` for money in
  minor units, `numeric(9,6)` for rates, `text` + `CHECK` for enumerations.

---

## 9. Logging and telemetry

```go
// Allowed.
slog.InfoContext(ctx, "plan computed",
    "corr", correlationID(ctx),
    "loans", len(pf.Loans),
    "strategy", plan.Strategy,
    "duration_ms", took.Milliseconds())

// Blocked by lint and stripped by the handler.
slog.Info("plan computed", "balance", state.Principal, "chat", chatID)
```

- Structured `slog` only. No `fmt.Print*`, no `log.Print*` — both are forbidden
  by lint.
- The redacting handler drops any attribute whose key is on the denylist
  (`balance`, `principal`, `payment`, `amount`, `chat`, `telegram_id`, `token`,
  `secret`) and any value of type `money.Amount`.
- Request bodies are never logged. A parse failure logs the field name and the
  failure kind, never the input.
- **No user, loan, or chat identifier in a metric label.** Internal UUIDs are
  pseudonymous identifiers, not operational data; ordinary telemetry carries a
  short-lived correlation ID instead.
- Metric labels must be bounded by construction: `locale`, `strategy`,
  `result`, `error_code`, and bucketed counts. If a label's cardinality depends
  on user growth, it is wrong.

---

## 10. Testing

| Layer | Tool | Proves |
| --- | --- | --- |
| Correctness corpus | `internal/corpus` replays `testdata/golden` | The engine matches a real lender to the minor unit — currently 59 of the 59 non-final rows of an Inecobank loan agreement. The only test that proves correctness rather than self-consistency. |
| Property tests | `testing/quick` | Principal components sum to the original; closing balance is exactly zero; a prepayment never increases total interest. |
| Ledger replay | table + property | `replay(contract, snapshot, events) == loan_state`, including under concurrent writes. |
| Unit | stdlib | Everything in `pkg/core`, with no fakes needed because there is no I/O. |
| Integration | `testcontainers-go` | Real Postgres: idempotency, `SKIP LOCKED` under contention, lease expiry, migration reversibility. |
| Bot journeys | fake transport + fake clock | Whole conversations end to end without a network. |
| Load | `k6` | Burst behaviour, not average throughput. |

Rules:

- Table-driven by default. One `t.Run` per case, named for the case.
- **`-race` always**, locally and in CI.
- **No `time.Sleep` in a test.** Ever. Inject a fake clock or wait on a channel.
  A sleeping test is a flaky test with a delay fuse.
- **No mock frameworks.** Hand-written fakes in `internal/app/apptest`. A fake
  you can read is worth more than a mock you have to configure.
- Assertions compare whole structs where practical, so a new field cannot be
  silently untested.
- **Every user-reported calculation discrepancy becomes a golden fixture before
  the fix is written.** The corpus is the asset.
- Tests may not reach the network. CI runs with the network disabled except for
  the container registry.

---

## 11. The five invariants

These are the project's actual constraints. Everything else is style.

| # | Invariant | Enforced by |
| --- | --- | --- |
| I1 | Money is never a float | `depguard` bans `float64` outside `rates`; review |
| I2 | `time.Now()` exists only in `sysclock` | `forbidigo`, package-scoped |
| I3 | `pkg/core` performs no I/O | `depguard` + the core-only test binary |
| I4 | The ledger is append-only and is the only truth | migration guard + generated-code test + nightly replay |
| I5 | No identifier or amount reaches a log or a metric label | redacting handler + `forbidigo` + a test that asserts a populated `LoanState` logs no digits of its balance |

A pull request that weakens one of these needs the invariant deleted from this
table first, in its own commit, with the reasoning. That is deliberately
annoying.

---

## 12. Tooling

| Tool | Purpose |
| --- | --- |
| `gofumpt` | Formatting. Stricter than `gofmt`, and not negotiable. |
| `golangci-lint` | Everything in §11 plus the usual suspects. |
| `sqlc` | Typed queries from `queries/*.sql`. |
| `goose` | Embedded, expand-only migrations. |
| `go-i18n` | Message catalogues with plural forms. |
| `testcontainers-go` | Real Postgres in integration tests. |
| `govulncheck` | Known-vulnerable dependency is a build failure. |

The full configuration lives in [`../.golangci.yml`](../.golangci.yml). The
parts that encode this document rather than general Go hygiene:

```yaml
version: "2"
linters:
  settings:
    depguard:
      rules:
        core-is-pure:
          files: ["**/pkg/core/**"]
          deny:
            - pkg: "database/sql"
              desc: "pkg/core performs no I/O (I3)"
            - pkg: "net/http"
              desc: "pkg/core performs no I/O (I3)"
            - pkg: "os"
              desc: "pkg/core performs no I/O (I3)"
            - pkg: "math/rand"
              desc: "pkg/core is deterministic"
        core-does-not-know-the-app:
          files: ["**/pkg/core/**"]
          deny:
            - pkg: "marumbot/internal"
              desc: "the engine is publishable alone; it must not import the application"
        app-does-not-know-adapters:
          files: ["**/internal/app/**"]
          deny:
            - pkg: "marumbot/internal/adapter"
              desc: "app declares interfaces; adapters implement them"
    forbidigo:
      analyze-types: true
      forbid:
        - pattern: '^time\.Now$'
          msg: "time enters through the Clock port; see internal/adapter/out/sysclock (I2)"
        - pattern: '^fmt\.Print.*$'
          msg: "structured logging only"
        - pattern: '^log\.(Print|Fatal|Panic).*$'
          msg: "use slog"
```

`sysclock` and `main` carry a narrow `//nolint:forbidigo` with a reason. A
`//nolint` without a reason is rejected by review.

---

## 13. Git

- **Trunk-based.** Short-lived branches off `main`, named `type/short-slug`
  (`fix/lease-expiry`, `feat/allocation-policy`).
- **Conventional Commits.** `feat:`, `fix:`, `refactor:`, `test:`, `docs:`,
  `chore:`, `perf:`. Subject ≤ 50 characters, imperative mood, no trailing
  period. The body explains *why*, and only when the why is not obvious.
- **DCO sign-off** (`git commit -s`), not a CLA.
- Commits are atomic: a schema change and the query change that needs it land
  together; an unrelated rename does not ride along.
- **Never rewrite published history** on `main`.
- The default branch is protected: review required, CI green, linear history.

### Pull requests

Small enough to review properly — if it is over roughly 400 changed lines,
there is usually a mechanical part that could have been its own commit.

The description answers three questions: what changes, why now, and how it was
verified. A PR touching `pkg/core` must state which golden fixtures cover it.

Reviewers check, in order:

1. Does it violate one of the five invariants?
2. Is a business rule leaking into an adapter?
3. Is there a new failure mode without a test?
4. Does it persist something recomputable?
5. Is the naming honest — does `updateBalance` update the balance and nothing else?

Formatting is never a review comment. That is the linter's job, and if the
linter does not catch it, the answer is a linter rule, not a comment.

---

## 14. Definition of done

- [ ] Behaviour covered by a test at the right layer, and the test fails without the change
- [ ] `make lint test` green locally, with `-race`
- [ ] No new `//nolint` without a written reason
- [ ] Migrations are expand-only, and the previous binary still works against them
- [ ] No new persisted state that could be recomputed
- [ ] No new metric label whose cardinality grows with users
- [ ] User-visible strings exist in both `en.toml` and `hy.toml`
- [ ] Errors do not leak amounts, identifiers, or secrets
- [ ] If it touches money, a golden fixture covers it
- [ ] If it touches delivery, the at-least-once behaviour still holds

---

## 15. Working with AI coding agents

`AGENTS.md` at the repository root is the short version of this document —
structure, the five invariants, the commands, and what needs human judgement.
Keep it under a page; agents read it every session and a long file crowds out
the actual task.

The invariants exist precisely because an agent will otherwise produce
reasonable-looking code that stores a float, calls `time.Now()` in the engine,
or logs a balance. Lint catches those on the first run rather than on review.

Two things an agent should never decide alone: a change to the five invariants,
and any number that appears in front of a user without a golden fixture behind
it.

---

## 16. Deliberately not rules

Left to the author. Do not raise these in review.

- Line length, as long as `gofumpt` is satisfied
- Whether to use a named return
- `var x = ...` versus `x := ...` at package scope
- Where to put a small helper — top or bottom of the file
- Test file organisation, as long as cases are named
- Comment style, beyond doc comments on exported identifiers
- Whether an interface has one implementation today

If one of these starts causing repeated argument, it graduates into §11 with a
lint rule attached, or it stays here and everyone stops bringing it up.
