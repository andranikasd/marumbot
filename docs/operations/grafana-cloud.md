# Grafana Cloud

Marum exports traces, metrics and logs over OTLP and profiles through
Pyroscope. Dashboards are deployed separately through Grafana's HTTP API.
Locally Compose supplies the OTel Collector and Pyroscope URLs; deployed dev
uses configured Cloud credentials. No production environment exists.

## What goes where

| Signal | Transport | Credential |
| --- | --- | --- |
| Traces | OTLP | `OTEL_EXPORTER_OTLP_HEADERS` |
| Metrics | OTLP | same |
| Logs | OTLP | same |
| Profiles | Pyroscope push | **its own URL, user and token** |
| Dashboards | Grafana HTTP API | a Grafana service account token |

## OTLP: traces, metrics and logs

```
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-gateway-<region>.grafana.net/otlp
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic <base64(instanceID:glc_token)>
```

The header is a **`key=value` list**, not a bare base64 string. Sent bare it
does not provide the required Authorization header and export can fail. Inspect exporter errors as well as the Cloud UI.

Verify a credential without deploying anything:

```bash
curl -i -X POST "$OTEL_EXPORTER_OTLP_ENDPOINT/v1/traces" \
  -H "Authorization: Basic $B64" -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[]}'
```

`B64` must contain the base64 encoding of the OTLP instance ID and token,
separated by a colon. A 2xx response accepts the probe; 401 indicates rejected
authentication. This probe does not verify actual application ingestion.

## Profiles: a second credential, from a different page

The application sends profiles with the Pyroscope client, not through its
OTLP exporters. Use the stack's separate profiling credentials.

Get the values from **Grafana Cloud → your stack → Details → Pyroscope → Send
Profiles**, and copy all three:

```
PYROSCOPE_SERVER_ADDRESS       https://profiles-prod-<NNN>.grafana.net
PYROSCOPE_BASIC_AUTH_USER      the numeric instance ID
PYROSCOPE_BASIC_AUTH_PASSWORD  a glc_ token with the profiles:write scope
```

Two things here reliably waste an afternoon:

**The hostname is numbered, not region-named.** It is `profiles-prod-001`, not
`profiles-prod-eu-west-6`. Knowing the OTLP endpoint tells you nothing about the
profiles one — they are different hostname families. Copy it, do not derive it.

**The user is the Pyroscope instance ID, which is not the OTLP instance ID.**
Every Grafana Cloud service tile publishes its own. Read it off the Pyroscope
tile.

An empty `PYROSCOPE_SERVER_ADDRESS` disables profiling. The profiler starts
after OTLP initialization, so an empty OTLP endpoint or an initialization
failure also prevents it from starting.

That is deliberate — profiling should never be the reason a deploy fails — but
it does mean a missing value looks exactly like a working one until you open the
profiles tab and find it empty.

## An empty service graph

If the service graph is empty while traces arrive, inspect metrics-generation
settings under Application Observability. This repository does not provision
those Cloud settings. `internal/obs/component.go` emits paired CLIENT and SERVER
spans with `peer.service`; confirm that the stack generates the needed metrics
from those spans. Local Tempo's generator is configured separately in
`deploy/observability/tempo.yaml`.

## Attributes Application Observability needs

`job` is mandatory there, and it is derived rather than set:
`service.namespace/service.name`, or just `service.name` when there is no
namespace. Marum sets both, plus the environment under two names:

```
service.name=marum
service.namespace=marum
deployment.environment=dev
deployment.environment.name=dev
```

The Worker supplies both environment spellings in `OTEL_RESOURCE_ATTRIBUTES`;
Go merges the default resource and explicitly sets `deployment.environment.name`.
Component tracers use `service.namespace=marum` and service names such as
`marum-scheduler`, `marum-store` and `marum-admin`. Keep service names free of
slashes so the namespace/name grouping stays unambiguous.

## Dashboards

The files in `deploy/observability/grafana/dashboards/` are the source of truth
for both places they appear. `docker compose` provisions them from disk; the
**Dashboards** workflow pushes them on a `main` push touching the dashboard
directory or that workflow. PRs validate only. Manual dispatch also validates
only: its Push step requires `github.event_name == 'push'`.

They bind datasources through `${ds_prometheus}`, `${ds_loki}` and `${ds_tempo}`
variables rather than by uid. A uid is local to one Grafana, so a dashboard that
names one directly renders as a wall of "datasource not found" in the other
instance even while the data arrives perfectly.

Every dashboard needs a stable `uid`. It is what makes a push an update instead
of a new dashboard, so a missing one multiplies dashboards on every run. CI
rejects a file without one, and rejects a hard-coded local datasource uid.

The workflow runs the repository's Python publisher. To publish manually,
provide `GRAFANA_URL` and `GRAFANA_TOKEN` in the environment and run from the
repository root:

```bash
python3 deploy/observability/push-dashboards.py
```

No `gcx` installation or `GRAFANA_STACK_ID` is used.

It authenticates with a **Grafana service account token** — created inside the
Grafana instance — not a Cloud access policy token. The two are not
interchangeable, and using the wrong one gives an unhelpful error.

| Setting | Where |
| --- | --- |
| `GRAFANA_URL` | repository variable, e.g. `https://<stack>.grafana.net` |
| `GRAFANA_TOKEN` | repository secret; service account token |

## Deploy annotations

Deployment annotations use the **dev environment's** `GRAFANA_URL` variable and
`GRAFANA_TOKEN` secret. This is separate from the repository-level dashboard
settings. Use a stack service-account token with annotation write access, not
an OTLP or Cloud access-policy credential.

The verified v2.0.3 deploy succeeded with an optional annotation warning:
HTTP 401, invalid API key. `Annotate Grafana` has `continue-on-error: true`;
fix its credential independently of deployment. An empty `GRAFANA_URL` skips
the annotation. It is not evidence of an application or database outage.

## Capacity

Check the stack's actual plan limits and ingestion usage when sizing telemetry;
no fixed free-tier allowance is guaranteed by this repository. Histogram
buckets multiply series by label combinations. Keep metric labels bounded and
never put balances, chat IDs or user UUIDs in logs or metric labels.
