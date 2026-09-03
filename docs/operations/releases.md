# Releasing

Tags trigger Release builds and GitHub artifacts, **not production deployment**.
Dev deploys from main pushes (excluding docs-only/ignored paths), or through
manual `cd-dev.yml` dispatch. No production environment exists.

## Verified baseline

Operator-verified state supplied for this audit (2026-09-03):

| Item | Verified value |
| --- | --- |
| Dev application version / tag | `2.0.3` / `v2.0.3` |
| Commit | `8d34606852f5d88ef31b8b32df757e37f0cce203` |
| Integration | PR #99 merged to `main` |
| Database schema | 22 |
| Planning engine | `plan/5` |
| Dev CD | run `33774632460`, success |
| Tag Release | run `33773187762`, success |
| CI | run `33773084763`, success |

The optional Grafana annotation returned HTTP 401 (`Invalid API key`); deployment
worked. These are verified baseline facts, not a claim that this document polls
live infrastructure.

## Versioning

Use `vMAJOR.MINOR.PATCH`: MINOR for user-visible features, PATCH for fixes,
MAJOR for breaking `pkg/core` API changes. The Release parser also accepts a
hyphenated prerelease suffix; `0.x` and suffixed versions are marked prereleases.

Automatic dev builds use the next patch after the highest stable tag plus
`-dev.<short-sha>`. Manual dev `version` accepts only `MAJOR.MINOR.PATCH`, without
`v` or a suffix. It stamps the selected ref; it does not check out the tag or
check that the version belongs to that commit.

The application version and engine version are distinct. The planning engine
constant in `pkg/core/plan/search.go` is `plan/5`, recorded in certificates and
plan manifests; it is not stamped from the release tag. Ledger replay accepts
its engine version as input. Preserve both code revision and recorded engine
metadata when investigating a number.

## Cutting a new release

After the intended code is merged and CI passes, choose an unused version:

```bash
git switch main
git pull --ff-only
VERSION=2.0.4  # example next patch; choose the intended unused version
git tag -a "v$VERSION" -m "Release $VERSION"
git push origin "v$VERSION"
```

Release then:

1. Validates the version and requires the tagged commit to be an ancestor of
   `origin/main`.
2. Runs `go test -race -count=1 ./...`.
3. Applies the new migrations to Postgres and runs the previous stable release's
   tests from a clean worktree. **This job sets `GOOSE_DBSTRING`, but not
   `TEST_DATABASE_URL`**: store tests requiring the latter skip. A green job
   alone does not prove previous-release store compatibility with the new schema.
4. Publishes amd64/arm64 GHCR images with SBOM, provenance and attestation.
5. Publishes GitHub Release notes grouped from Conventional Commits and the image
   digest. There is no deployment job chained after this.

CI separately runs store tests against a migrated Postgres with
`TEST_DATABASE_URL`, plus lint, vet/vulnerability checks, tests, Mini App and
smoke regressions, arm64 determinism, engine isolation, migration reversibility,
Worker checks and image build. Secret scanning is a separate workflow.

## Deploying an explicit release to dev

First verify the selected source equals the release tag. With local refs freshly
updated, the current baseline check and dispatch are:

```bash
test "$(git rev-parse main)" = "$(git rev-parse 'v2.0.3^{commit}')"
gh workflow run cd-dev.yml --ref main -f version=2.0.3
```

The workflow checks out remote `main`; ensure it has not advanced since that
comparison. If main has moved, do not stamp newer code as `2.0.3`. Use the normal
dev stamp or cut the appropriate new release. Tag-ref dispatch is rejected by
dev environment protection; retain the protection.

See [deployment.md](deployment.md) for infrastructure, expand migrations,
pre-release rollback target capture, secret sync and exact-version smoke.
`cd-prod.yml` remains manual future use only; do not treat it as a ready
promotion command.

## Commits and hotfixes

Use Conventional Commits, subject at most 50 characters, imperative mood and
no trailing period. Explain why in the body when needed; sign off with
`git commit -s`.

For a hotfix, branch from the appropriate tag, fix and merge to main before
pushing a PATCH tag. The tagged commit must be an ancestor of main; dev follows
the main source, not whichever tag was most recently published.
