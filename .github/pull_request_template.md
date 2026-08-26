## What changes

<!-- One or two sentences. The commit messages carry the detail. -->

## Why now

<!-- What made this worth doing. If it fixes something, what was broken. -->

## How it was verified

<!-- What you ran, and what it showed. "Tests pass" is not verification. -->

---

- [ ] `make lint test` green
- [ ] If it touches money, a golden fixture covers it
- [ ] If it adds a migration, it is expand-only and the previous release still works against it
- [ ] No new metric label whose cardinality grows with users
- [ ] User-visible strings exist in both `en.toml` and `hy.toml`
