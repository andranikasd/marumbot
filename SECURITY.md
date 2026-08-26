# Security

## Reporting

Report privately through
[GitHub Security Advisories](https://github.com/andranikasd/marumbot/security/advisories/new).
Never open a public issue for a vulnerability.

Expect an acknowledgement within 72 hours.

## What Marum holds

Loan balances, payment history, and an encrypted Telegram identifier. That is
enough to matter, which is why the design keeps it small:

- **Never collected**: bank credentials, card numbers, CVV codes, passport or
  social-card numbers. Input resembling any of those is rejected without being
  stored.
- **Separated**: Telegram identifiers live in their own table, encrypted with a
  versioned key, apart from every financial record.
- **Never logged**: amounts are stripped from telemetry *by type*, not by field
  name, so a new field cannot leak one by being named something unexpected.

## Scope

In scope: authentication and session handling, the webhook boundary, injection,
data exposure through logs or telemetry, the admin interface, and anything that
lets one account reach another's data.

Out of scope: the absence of a feature, denial of service through sheer volume
against a free-tier deployment, and anything requiring physical access to a
self-hosted instance.

## Self-hosting

Two settings fail closed and should stay that way:

- No `MARUM_ADMIN_PASSWORD_HASH` means the admin interface does not start,
  rather than starting unauthenticated.
- An empty OTLP endpoint disables telemetry entirely.

Never expose the admin interface publicly. It binds to loopback for a reason.
