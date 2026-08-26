# Releasing

Tags are the trigger and the source of truth. Nothing else creates a release,
so what is running can always be traced back to a tag.

```mermaid
flowchart LR
  pr["Pull request"] --> ci["CI: lint, vet, vuln,<br/>tests, migrations,<br/>engine purity, image"]
  ci --> main["Merge to main"]
  main --> tag["Tag vX.Y.Z"]
  tag --> rel["Release workflow"]
  rel --> ver["Validate SemVer<br/>and ancestry"]
  ver --> compat["Previous release<br/>vs this schema"]
  compat --> img["Multi-arch image<br/>SBOM + provenance"]
  img --> gh["GitHub Release<br/>generated notes"]
  gh --> dep["Deploy workflow"]
```

## Versioning

Semantic versioning, `vMAJOR.MINOR.PATCH`.

| Change | Bump |
| --- | --- |
| A user-visible feature | MINOR |
| A fix with no interface change | PATCH |
| A breaking change to `pkg/core`'s public API | MAJOR |
| Anything at all, while below 1.0.0 | MINOR for features, PATCH for fixes |

**Below 1.0.0 nothing is promised to be stable**, and the release workflow
marks every `0.x` tag as a pre-release so that is visible rather than assumed.

`pkg/core` is importable on its own and carries its own compatibility promise.
A change that would break an external importer is a MAJOR bump even if the bot
is unaffected.

### The engine version

`EngineVersion` is stamped from the tag and recorded on every plan and every
allocation result. A number shown to a user can always be traced to the code
that produced it, which is what makes a discrepancy report answerable months
later.

## Cutting a release

```bash
git switch main && git pull
git tag -a v0.3.0 -m "reminders and payment recording"
git push origin v0.3.0
```

The workflow then:

1. **Validates the tag** — strict SemVer, and it must be an ancestor of `main`.
   A tag that is not a version is a mistake, and a mistake that reaches a
   registry is one somebody has to chase.
2. **Runs the full suite** with the race detector.
3. **Checks backward compatibility** — applies this tag's migrations, then runs
   the *previous* release's tests against them. Migrations expand first, so the
   binary being replaced must keep working. This matters more than up/down/up:
   rolling back a binary is routine, rolling back a migration is not.
4. **Builds** a multi-arch image with an SBOM and a provenance attestation, and
   pushes it to GHCR.
5. **Publishes** a GitHub Release with notes grouped by Conventional Commit
   type, and the image digest.

## Commits

Conventional Commits. The release notes are generated from them, so a lazy
subject line becomes a bad changelog entry.

```
feat(core): add payment allocation and ledger replay
fix(obs): correlate logs with the span that produced them
```

Subject ≤ 50 characters, imperative mood, no trailing period. The body explains
**why**, and only when the why is not obvious. Sign off with `git commit -s`.

## Deploying

Deployment follows a **published release**, never a branch.

```
expand migration → deploy dual-schema code → smoke → (rollback on failure)
```

Rolling back the binary never requires rolling back the schema, because the
schema only ever expanded. A destructive contract migration happens in a later
release, once nothing reads the old representation.

Trigger manually for staging:

```
Actions → Deploy → Run workflow → version 0.3.0, environment staging
```

## Hotfixes

Branch from the tag, fix, tag a PATCH, and merge back to `main`. Do not tag off
a branch that is not an ancestor of `main` — the workflow refuses it, on
purpose.
