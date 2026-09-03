# Documentation

## Start here

**Current release: v2.0.4, development only** (verified 2026-09-03).
Start with [current state](current-state.md), [budgeting](product/budgeting.md),
[development acceptance](design/v3/development-acceptance.md) and
[release evidence](design/v3/release-checklist.md). There is no production environment.

New to the codebase? Read [architecture/01-overview.md](architecture/01-overview.md),
then [02-domain-model.md](architecture/02-domain-model.md). Between them they
explain why the code is shaped the way it is.

## Architecture

| Document | Covers |
| --- | --- |
| [System overview](architecture/01-overview.md) | The whole system on one page, and the rules that shape it |
| [Domain model](architecture/02-domain-model.md) | Contracts, snapshots, events, derived state, reliability |
| [Ledger and replay](architecture/03-ledger-replay.md) | How a balance is derived, and the double-counting trap |
| [Money and dates](architecture/04-money-and-dates.md) | Integer money, currencies, the 128-bit accrual, anchored schedules, and what the corpus settles about day count and rounding |
| [Database](architecture/05-database.md) | Schema, conventions, the constraints SQL cannot express |
| [Admin interface](architecture/06-admin-ui.md) | The private operator UI and its security model |
| [Messaging](architecture/07-messaging.md) | Telegram in and out, delivery guarantees, scheduling |
| [Observability](architecture/08-observability.md) | Five signals, correlation, seeing inside the monolith |

## Operations

| Document | Covers |
| --- | --- |
| [Local development](operations/local-development.md) | Getting running, every make target, the demo data |
| [Environments](operations/environments.md) | Live development, inactive production configuration and required settings |
| [Releasing](operations/releases.md) | Versioning, tags, the release pipeline |
| [Deployment](operations/deployment.md) | Cloudflare setup, secrets, the deploy order |
| [Infrastructure](../deploy/terraform/README.md) | Terraform: where the database lives, Hyperdrive, who owns which resource |
| [Grafana Cloud](operations/grafana-cloud.md) | The five signals, the two credentials, and the switch that is off by default |
| [Runbooks](operations/runbooks.md) | What to do when something is wrong |

## Reference

| Document | Covers |
| --- | --- |
| [Correctness corpus](../testdata/golden/README.md) | The real lender schedules the engine is measured against, and what each one currently proves |
| [Engineering guide](engineering-guide.md) | How code is written: structure, style, testing, the five invariants |
| [Current interface conventions](design/ui-v1.1.md) | v2.0.4 navigation, tokens, localization and recovery; legacy filename |
| [MVP system and architecture design](design/Marum-MVP-System-and-Architecture-Design.pdf) | Archived original design; not current deployment evidence |
| [Reliable MVP design v0.3.1](design/reliable-mvp-design.md) | Historical proposal; use migrations/current architecture for implemented behavior |
| [Diagrams](diagrams/) | Archived draw.io figures and exports for the original design |

## Historical design and review records

These assets preserve original intent and review history. Their feature lists,
schema examples, diagrams and screenshots do not certify current implementation.

- [Original v3 specification](design/v3/specification.md)
- [Historical implementation review](design/v3/implementation-review.md)
- [Decision-engine review](design/Marum-Decision-Engine-Review-and-Definition-of-Done.md)
- [Original PDF source HTML](design/marum-final.html) and the PDF linked above
- [v3 visual preview](design/v3/preview.html), [three-loan preview](design/v3/three-loans.html)
  and [synthetic reference data](design/v3/three-loans-data.json)
- [v1.0.0 beta record](release/v1.0.0-beta.md)

## Governing constraints

[AGENTS.md](../AGENTS.md) retains the five invariants and human-review gates.
The [engineering guide](engineering-guide.md) distinguishes conventions from
actual automated enforcement. Financial facts are append-only, money is integer
minor units, core calculations are deterministic, and unknown lender behavior
must not be guessed. Original plan inputs and activation records are persisted
for history; derived reports are recomputed from them.
