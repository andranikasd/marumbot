# Admin interface

A server-rendered operator interface. The application bind defaults to `:8081`;
local Compose publishes it on `127.0.0.1:8081`, while development exposes the
authenticated interface at `https://admin-dev.marum.loan`.
Go `html/template` renders the pages with shared design tokens and no external
assets. This describes the development system; there is no production instance.

Main still leaves the listener disabled without `MARUM_ADMIN_PASSWORD_HASH`.
That configuration bootstraps an individual identity only when the registry is
empty; it is not a shared-password-only authentication system.

## Authentication and authorization

```mermaid
flowchart LR
  login["Individual password"] --> otp["TOTP enrollment or verification"]
  otp --> session["Random in-memory session"]
  session --> auth["Reload identity/version and roles"]
  auth --> purpose["Required purpose · recent step-up where required"]
  purpose --> audit["Persist access audit"]
  audit --> service["Authorized application operation"]
```

- Passwords use PBKDF2-HMAC-SHA256, 210,000 rounds. The bootstrap identity
  receives administrator only, with no implicit financial-read permission.
- First password login permits authenticator enrollment only. Enrollment proves
  a TOTP; subsequent logins require password and TOTP. PostgreSQL consumes OTP
  counters so a restart does not permit reuse.
- Sessions are random tokens held in memory, revoked on logout/restart and
  invalidated by identity version changes. They are not stateless HMAC cookies.
- The eight explicit roles are support reader, support operator, financial
  verifier, policy publisher, operations, billing operator, security auditor and
  administrator. Grants combine only when roles are assigned explicitly.
- Application services enforce authorization, purpose requirements, recent
  step-up and audit persistence. Financial reads/replay require a purpose;
  publication and role changes require step-up within five minutes. Missing
  authentication or failed audit writes deny access.
- Browser purpose lives in the session; API clients can supply `X-Admin-Purpose`.
  Self-role changes are denied. There is no impersonation or financial-fact
  overwrite API. Navigation reflects grants but does not replace service checks.
- Failed login attempts are throttled. The handler checks supplied write origins
  and serves a restrictive CSP (`default-src 'none'`) with same-origin styles
  and forms. The admin listener is separate from public borrower routes.

## Policy publication and management

A financial verifier creates a draft with source/evidence, and another identity
reviews its exact content hash. Editing clears review. Publication requires the
reviewed hash, publisher role, recent step-up and a signing key; the author
cannot publish. A reviewer may publish only with an explicit publisher role.
The draft transition and active policy insertion are atomic.

Main derives the signing key from the decoded identity key using HMAC-SHA256 and
`marum/admin-policy-signing/v1`. Publication fails closed without a signer.
Review/publication records provenance; it does not prove an unknown lender rule
or change the policy version pinned by existing contracts.

| Workspace | Purpose |
| --- | --- |
| Overview, commands, deliveries | Health, queue inspection and existing safe command retry. Public readiness/status use the separate non-personal `app.Operations` service. |
| Loans, users, reconciliation | Authorized borrower records and financial evidence. |
| Security, identities, audit | Purpose/step-up, individual access management, immutable access/security events. |
| Policies and corpus | Draft, independent review, signing/publication; embedded fixture provenance and recorded coverage. |
| Cases | Classified support cases with stored resolution evidence; support reads redact free text/evidence. |
| Profile flags | Versioned environment/profile planning switches with reasons. |
| History | Original manifests and exact replay; unavailable historical engines return a visible refusal. |
| Entitlements | Access/trial/deletion state and pause/restore actions without financial data. |

## Rendering

The stylesheet prepends [shared tokens](../../internal/design/tokens.css) to the
admin sheet. Each page has its own template set containing the layout and
itself; sharing all `content` definitions would make the last parsed page win.
Rendering uses a buffer so a template error cannot leave a partial 200 response.

## Evidence and remaining limits

See the [adapter README](../../internal/adapter/in/admin/README.md) for routes,
wiring and gaps, [authentication implementation](../../internal/adapter/in/admin/security.go),
[application authorization](../../internal/app/admin_security.go),
[role grants](../../internal/app/admin_access.go), and
[policy transitions](../../internal/app/admin_policy.go).

Recovery/reset, key-rotation UI, full export/pagination, richer policy lifecycle
metadata and additional billing/support workflows remain incomplete. Corpus
pages show committed evidence, not a fresh execution or independent attestation
of the golden suite. The [development acceptance bounds](../design/v3/development-acceptance.md)
remain the limit of the claims made here.
