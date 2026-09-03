# Admin security integration

Audited against development **v2.0.3**, commit
`8d34606852f5d88ef31b8b32df757e37f0cce203`, schema **22**, engine **`plan/5`**.
There is no production deployment. See the
[architecture](../../../../docs/architecture/06-admin-ui.md) and
[development acceptance bounds](../../../../docs/design/v3/development-acceptance.md).

Apply the current migration set before starting admin. Migration 00015 introduced
security identities, policy/control revisions and audit records; its Down
preserves them. History/scenarios and command receipts use later migrations.

Current [main wiring](../../../../cmd/marum/main.go):

```go
svc := app.NewAdmin(store).
    WithModeration(store).
    WithEngine(store).
    WithSecurity(store, clock.Now).
    WithHistory(store)
server, err := admin.New(svc, admin.Config{
    User: configuredAdminUser,
    PasswordHash: configuredAdminPasswordHash,
    PolicySigningKey: policySigningKey,
    Now: clock.Now,
    // Keep existing Version and Env fields.
}, logger)
```

`PolicySigningKey` is a stable secret of at least 32 bytes. Publication fails
closed without it; login/enrollment still work. The application derives it from the decoded identity key using HMAC-SHA256
and the domain `marum/admin-policy-signing/v1`; deployment needs no new secret.
HMAC-SHA256 signatures bind the canonical policy payload and evidence. Retain
old keys for external verification when rotating; no key-rotation UI is supplied.

The configured user/password hash seeds one identity only when the registry is
empty. It receives **only administrator**. Password login initially permits
only authenticator enrollment; enrollment must prove a valid TOTP. Each further
login requires password plus TOTP. OTP counters are consumed in PostgreSQL,
including across process restarts. Sessions are random, in memory, revoked on
logout/restart, and invalidated by identity version changes. Credentials/roles
subsequently come from the database, not repeated config overwrites.

Open `/security` to set an access purpose and perform recent step-up. Browser
purposes stay in the session; API clients may override with `X-Admin-Purpose`.
Application services reload roles, audit access before reading data, and deny
missing authentication, audit failures, missing purposes and stale step-up.
The eight roles are explicit unions, with no administrator financial grant.
Self-role changes are denied: create a second administrator before changing the
bootstrap account. No account impersonation or financial-fact editing API exists.

Actual management APIs (cookie authentication):

- `POST /api/identities`: `ID`, `Username`, `Password`, `Roles`, `Enabled`,
  `ExpectedVersion` (0 creates). Updates increment the identity version and
  preserve its enrolled authenticator. Administrative changes require step-up.
- `POST /api/policies`: `Policy`, `ExpectedVersion`. Policy fields: `ID` (UUID),
  `Key`, `Version`, `Definition`, `Excess`, `Source`, `Evidence`. Creates/edits a
  draft only; editing clears its review.
- `GET /api/policies/{id}`: reads the draft/review and content hash (verifier or publisher; purpose required).
- `POST /api/policies/{id}/review` or `/publish`: `ExpectedVersion`,
  `ContentHash`. Review cannot be by the author; publication requires the
  reviewed hash, publisher role, recent step-up and configured signing key.
  The author cannot publish. A verifier may publish only with an explicit
  publisher role. Draft transition and active policy insertion are atomic.
- `GET /api/audit`: security auditor; returns the latest 200 immutable events.
- `POST /api/cases`: `Case`, `ExpectedVersion`. `Case` has `ID`, `UserID`,
  `LoanID`, `Category`, `Note`, `State`, `Resolution`, `EvidenceID`. Closing needs
  a classified resolution and a stored anchor, void/replacement pair, or
  published independently reviewed policy. SQL verifies evidence existence.
- `GET /api/cases/{id}`: redacted classification/status; omits free text/evidence.
- `POST /api/profile-flags`: `Flag`, `ExpectedVersion`. `Flag` has `Environment`,
  `Profile`, `PlanningEnabled`, `Reason`.
- `GET /api/users/{id}/history`: financial verifier, purpose required.
- `POST /api/users/{id}/history/{report}/replay`: both safe-replay and financial
  read capabilities, purpose required. Calls `ReplayManifest` with original
  stored versions; `ErrHistoricalEngine` is a visible 409, never latest-rule replay.

Management pages (no JSON authoring required):

- `/security`: session purpose, recent step-up, role-relevant workspace links.
- `/identities`: credential-free directory, individual creation and role/access
  editing. Blank password on an edit preserves the current password.
- `/policies`: registry; `/policies/new` and `/policies/{id}` create/edit source
  and evidence, independently approve the exact hash, and sign/publish it.
- `/cases` and `/cases/{id}`: classified cases, append notes, and choose stored
  resolution evidence. Support reads redact notes; financial reads require purpose.
- `/flags`: versioned environment/profile switches with mandatory reasons.
- `/audit`: latest 200 immutable access/security events.
- `/history` and `/users/{id}/history`: original manifest versions and exact
  report replay, with first-cycle actions and the complete replay timeline.
- `/entitlements`: account access/trial/deletion state without loan data; existing
  pause/restore actions. `/deliveries` and `/commands` retain notification and queue
  read workflows and the existing safe command retry.
- `/corpus`: embedded golden manifest, source fields and schedule rows, recorded
  support states, and copyable fixture/hash evidence references. Its provenance
  test requires the embedded snapshot to match `testdata/golden` exactly.

All form actions call the same application authorization and optimistic-lock
boundaries as the APIs. The role check also controls navigation, but hiding a
button never grants or removes application authority. New directory queries use
the security tables introduced in migration 00015; the complete application
uses schema 22.
`admin.New` attaches the embedded corpus automatically.

Remaining product gaps (do not mark the full admin specification complete):

- Unknown lender/product definitions are not implemented or automatically trusted.
  Publication records reviewed versions; existing contracts stay pinned.
- No authenticator recovery/reset or signing-key rotation interface.
- Corpus metadata reports committed results; this UI does not run/attest the
  complete golden suite or replace independent policy verification.
- Full policy lifecycle metadata (experimental/provisional/disabled/superseded),
  advanced financial dashboards and before/after diffs remain unimplemented.
- Lists/audit are capped at 200 rather than paginated; no full audit export or
  export-verification workflow. Moderation audit records attempts, not completion.
- No provider billing status or trial-extension workflow; no new delivery retry
  beyond existing command retry. Case user-notification/confirmation remains absent.
- A publisher or verifier cites source evidence manually; a fixture reference
  does not promote trust or prove lender-wide coverage.

Replay retains original manifests, not merely a pointer to today's loan/budget
state. [PlanManifest](../../../app/plan_manifest.go) contains the source identity,
original input, goal, selected policy, schema/engine and budget versions, and
input/result hashes. Projection rows are recomputed and verified; mismatched
hashes conflict and an unavailable engine refuses. Original inputs/policies and
activation history are persisted for audit, so “plans are never persisted” is
incorrect. Previewed/saved scenarios do not activate a plan.

Source evidence: [session/TOTP handling](security.go),
[authorization and audits](../../../app/admin_security.go),
[role capabilities](../../../app/admin_access.go),
[review/publication](../../../app/admin_policy.go),
[historical replay](../../../app/admin_history.go), and
[PostgreSQL history](../../out/postgres/plan_history.go).
