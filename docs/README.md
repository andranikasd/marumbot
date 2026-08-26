# Documentation

## Start here

New to the codebase? Read [architecture/01-overview.md](architecture/01-overview.md),
then [02-domain-model.md](architecture/02-domain-model.md). Between them they
explain why the code is shaped the way it is.

## Architecture

| Document | Covers |
| --- | --- |
| [System overview](architecture/01-overview.md) | The whole system on one page, and the rules that shape it |
| [Domain model](architecture/02-domain-model.md) | Contracts, snapshots, events, derived state, reliability |
| [Ledger and replay](architecture/03-ledger-replay.md) | How a balance is derived, and the double-counting trap |
| [Money and dates](architecture/04-money-and-dates.md) | Integer money, currencies, the 128-bit accrual, anchored schedules |
| [Database](architecture/05-database.md) | Schema, conventions, the constraints SQL cannot express |
| [Admin interface](architecture/06-admin-ui.md) | The private operator UI and its security model |
| [Messaging](architecture/07-messaging.md) | Telegram in and out, delivery guarantees, scheduling |
| [Observability](architecture/08-observability.md) | Five signals, correlation, seeing inside the monolith |

## Operations

| Document | Covers |
| --- | --- |
| [Local development](operations/local-development.md) | Getting running, every make target, the demo data |
| [Environments](operations/environments.md) | dev and production: what differs, what is enforced, what to set |
| [Releasing](operations/releases.md) | Versioning, tags, the release pipeline |
| [Deployment](operations/deployment.md) | Cloudflare setup, secrets, the deploy order |
| [Infrastructure](../deploy/terraform/README.md) | Terraform: where the database lives, Hyperdrive, who owns which resource |
| [Runbooks](operations/runbooks.md) | What to do when something is wrong |

## Reference

| Document | Covers |
| --- | --- |
| [Engineering guide](engineering-guide.md) | How code is written: structure, style, testing, the five invariants |
| [MVP system and architecture design](design/Marum-MVP-System-and-Architecture-Design.pdf) | The formal design document |
| [Reliable MVP design v0.3.1](design/reliable-mvp-design.md) | Long-form reference: full DDL, reliability invariants, failure analysis |
| [Diagrams](diagrams/) | Editable draw.io sources for the design document's figures |

## The short version

If you read nothing else:

1. **Money is `int64` minor units.** Interest accrual runs through a 128-bit
   intermediate, because the naive form overflows above ~16.5M AMD at 18% and
   fails **silently**.
2. **Facts are append-only.** A mistake is superseded or voided, never edited.
3. **Derived state is disposable.** Delete `loan_state`; replay rebuilds it.
4. **Delivery is at-least-once.** Reminder text must read correctly twice.
5. **When Marum cannot reconstruct a balance it trusts, it says so** and asks
   for the bank's figure rather than guessing.
