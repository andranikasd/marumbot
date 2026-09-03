# Environments

**Dev is the only deployed environment. No production exists.** The dormant
`cd-prod.yml` is manual future use only; its GitHub and Wrangler environment
name is `prod`, not `production`. It is not a working promotion path: production
backend/tfvars and complete Worker/container/database wiring do not exist, and
the reusable deploy currently selects `TF_DATABASE_DEV`.

| Setting | Current dev |
| --- | --- |
| GitHub / Wrangler environment | `dev` / `dev` |
| Worker | `marum-dev` |
| Public domain | `dev.marum.loan` |
| Admin domain | `admin-dev.marum.loan` |
| Container | `MarumApp`, singleton, `basic`, maximum 1 instance |
| Database | Neon, reached directly by the container |
| Hyperdrive | `marum-dev-postgres`, bound but unused by the container |
| Cron / container idle timeout | every 5 minutes / 10 minutes |
| Automatic deploy | push to `main`, except changes only to `docs/**`, `**/*.md`, `LICENSE`, `NOTICE` |

Keep the local polling bot separate from the deployed dev webhook bot. Any
future environment also needs its own bot, database and runtime secrets.

## Deployment refs and versions

Dev deploys automatically without a reviewer. Its environment protection rejects
tag refs; dispatch from `main` and retain that protection. The version input
changes the build stamp, not the source checkout. To deploy a release version,
first ensure `main` and the tag identify the same commit, then use:

```bash
gh workflow run cd-dev.yml --ref main -f version=2.0.3
```

Without an explicit input, CD computes the next patch after the highest stable
tag and adds `-dev.<short-sha>`. Pushing a version tag runs **Release**, which
builds images and publishes artifacts/notes; it does not deploy production.
See [releases.md](releases.md) for the verified version and commit.

Repository workflows define CI jobs, not the complete live branch-protection
policy. Do not infer a fixed number of required checks from an old runbook.

## Secrets and variables

Set deployment settings in the GitHub **dev environment**:

| Kind | Name | Purpose |
| --- | --- | --- |
| Secret | `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` | Worker/container deployment |
| Secret | `DATABASE_URL` | Goose migration connection |
| Secret | `MARUM_DATABASE_URL` | Container's direct Postgres connection; not the Hyperdrive connection string |
| Secret | `MARUM_BOT_TOKEN` | Dev bot |
| Secret | `MARUM_WEBHOOK_SECRET`, `WEBHOOK_PATH` | Telegram secret header and high-entropy URL path |
| Secret | `MARUM_SERVICE_TOKEN` | Worker-to-container authentication |
| Secret | `MARUM_IDENTITY_KEY` | Base64-encoded 32-byte identity encryption key |
| Secret | `MARUM_MINIAPP_URL` | `https://dev.marum.loan/app/`; required by the deploy's secret sync |
| Secret | `MARUM_ADMIN_USER`, `MARUM_ADMIN_PASSWORD_HASH` | Admin login; absent hash disables listener |
| Secret | `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS` | Optional telemetry |
| Secret | `PYROSCOPE_SERVER_ADDRESS`, `PYROSCOPE_BASIC_AUTH_USER`, `PYROSCOPE_BASIC_AUTH_PASSWORD` | Optional profiling |
| Secret | `GRAFANA_TOKEN` | Optional deploy annotation, stack service-account token |
| Variable | `PUBLIC_URL` | Smoke base URL, `https://dev.marum.loan` |
| Variable | `GRAFANA_URL` | Optional Grafana stack URL |

The deploy bulk-syncs nonempty runtime secrets **after** deploying. Empty values
are omitted, so clearing a GitHub value does not remove an existing Worker
secret. The Worker supplies `MARUM_MODE=webhook`, `MARUM_ENV=dev`, the build
version and instance ID to the container.

Infrastructure uses **repository secrets**: `TF_CLOUDFLARE_API_TOKEN`,
`TF_CLOUDFLARE_ACCOUNT_ID`, `TF_CLOUDFLARE_ZONE_ID`, `TF_R2_ACCESS_KEY_ID`,
`TF_R2_SECRET_ACCESS_KEY`, and `TF_DATABASE_DEV`. PR plans do not declare a
GitHub environment; manual Infrastructure runs and deployments do. Only dev
has committed Terraform environment files. Dev state owns the zone TLS/HSTS
settings. See [Terraform](../../deploy/terraform/README.md).

Dashboard publishing separately reads repository-level `GRAFANA_URL` (variable)
and `GRAFANA_TOKEN` (secret); it does not use the dev environment's settings.
