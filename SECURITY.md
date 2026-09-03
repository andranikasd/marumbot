# Security

## Reporting

Report privately through
[GitHub Security Advisories](https://github.com/andranikasd/marumbot/security/advisories/new).
Never open a public issue for a vulnerability.

Expect an acknowledgement within 72 hours.

## What Marum holds

Marum stores loan terms and balances, payment/statement history, budget and
funding declarations, plan source/history records and account preferences.
Telegram identifiers are encrypted and kept separately from financial records.
Operator identities and authentication/audit records have separate controls.

Do not submit bank credentials, card security codes or identity documents.
Input screening is not a guarantee that every sensitive free-text value will be
recognized. Never include real account data or secrets in public bug reports.
Amounts and identifiers must stay out of telemetry. Typed-amount and sensitive-key
redaction provide additional protection, not a substitute for safe logging.

## Scope

In scope: authentication and session handling, the webhook boundary, injection,
data exposure through logs or telemetry, the admin interface, and anything that
lets one account reach another's data.

Out of scope: the absence of a feature, denial of service through sheer volume
against a free-tier deployment, and anything requiring physical access to a
self-hosted instance.

## Self-hosting

With no `MARUM_ADMIN_PASSWORD_HASH`, the admin interface is not started.
When enabled, the password is part of bootstrap; individual operator identities,
TOTP, roles and purpose/step-up checks govern access. Follow the
[admin guide](internal/adapter/in/admin/README.md).

The application admin bind address defaults to `:8081`, not loopback. Local
Compose publishes it on loopback; hosted development routes a separate admin
hostname to that port. Self-hosters must configure their network boundary and
HTTPS deliberately, and must not expose an unprotected container port.

An empty OTLP endpoint disables configured OTLP export; redacted stdout logging
remains available. Review [observability](docs/architecture/08-observability.md)
and [deployment](docs/operations/deployment.md) before enabling external export.
The current hosted environment is development only.
