# Grafana Cloud

Marum sends five signals to one place. Development and production differ only in
the endpoint: locally it is the OTel Collector in `docker compose`, deployed it
is Grafana Cloud.

Four of the five work with the OTLP credentials alone. Profiles need a second,
separate credential, and the service graph needs a setting turned on that is off
by default — those two are why a stack can look half empty while everything is
configured "correctly".

## What goes where

| Signal | Transport | Credential |
| --- | --- | --- |
| Traces | OTLP | `OTEL_EXPORTER_OTLP_HEADERS` |
| Metrics | OTLP | same |
| Logs | OTLP | same |
| Profiles | Pyroscope push | **its own URL, user and token** |
| Dashboards | `gcx` | a Grafana service account token |

## OTLP: traces, metrics and logs

```
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-gateway-<region>.grafana.net/otlp
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic <base64(instanceID:glc_token)>
```

The header is a **`key=value` list**, not a bare base64 string. Sent bare it
becomes a malformed header name and every signal is dropped in silence — the
gateway answers `401` and nothing in the application notices.

Verify a credential without deploying anything:

```bash
curl -i -X POST "$OTEL_EXPORTER_OTLP_ENDPOINT/v1/traces" \
  -H "Authorization: Basic $B64" -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[]}'
```

`200` or `204` is accepted. `401` means the instance ID and token do not match —
try the same call with a deliberately wrong instance ID, and if that also
returns `200` you are not testing what you think you are.

## Profiles: a second credential, from a different page

Profiles do **not** travel over OTLP. Grafana Cloud's OTLP endpoint documents
exactly three signals — `/v1/metrics`, `/v1/logs`, `/v1/traces` — and profiles is
not among them. The OpenTelemetry profiles signal is still changing, so this is
not a gap that closes by waiting.

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

An empty `PYROSCOPE_SERVER_ADDRESS` disables profiling without a word:

```go
if cfg.PyroscopeAddr == "" { return }
```

That is deliberate — profiling should never be the reason a deploy fails — but
it does mean a missing value looks exactly like a working one until you open the
profiles tab and find it empty.

## The service graph is off until you turn it on

Grafana Cloud does not generate service-graph metrics from traces by default:

> Metrics-generation is disabled by default. You can enable it for use with
> Application Observability defaults in Application Observability, or contact
> Grafana Support to enable metrics-generation for your organization with custom
> settings.

Enable it under **Application Observability → Configuration → System**. No
quantity of correct tracing produces a graph before that switch.

One detail worth knowing after it is on: the default only generates metrics for
`SERVER` and `CONSUMER` spans. Edges in a service graph come from the client
side, which is exactly why `internal/obs/component.go` emits paired CLIENT and
SERVER spans with `peer.service` set — client-side generation has to be
configured separately for those edges to appear.

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

The environment is emitted twice on purpose: `deployment.environment` is what
Grafana still documents, `deployment.environment.name` is the current
OpenTelemetry semantic convention, and **baselines do not work without it**.

Neither `service.name` nor `service.namespace` may contain a slash — `/` is the
separator inside `job`, so a slash in either silently produces a different
service than intended.

## Dashboards

The files in `deploy/observability/grafana/dashboards/` are the source of truth
for both places they appear. `docker compose` provisions them from disk; the
**Dashboards** workflow pushes the same files to Grafana Cloud on merge to
`main`.

They bind datasources through `${ds_prometheus}`, `${ds_loki}` and `${ds_tempo}`
variables rather than by uid. A uid is local to one Grafana, so a dashboard that
names one directly renders as a wall of "datasource not found" in the other
instance even while the data arrives perfectly.

Every dashboard needs a stable `uid`. It is what makes a push an update instead
of a new dashboard, so a missing one multiplies dashboards on every run. CI
rejects a file without one, and rejects a hard-coded local datasource uid.

Tooling here changed recently, so to save the search: **`gcx` is current**.
`grafanactl` is deprecated and its repository was archived in June 2026; Grizzly
has been removed outright. Neither is safe to build on.

```bash
gcx resources push dashboards --path <file>
```

It authenticates with a **Grafana service account token** — created inside the
Grafana instance — not a Cloud access policy token. The two are not
interchangeable, and using the wrong one gives an unhelpful error.

| Setting | Where |
| --- | --- |
| `GRAFANA_URL` | repository variable, e.g. `https://<stack>.grafana.net` |
| `GRAFANA_STACK_ID` | repository secret |
| `GRAFANA_TOKEN` | repository secret; service account token |

## Free tier

Everything Marum sends fits, with one cap worth watching:

| | Free tier |
| --- | --- |
| Metrics | 10k active series, 14 days |
| Logs | 50 GB/month, 14 days |
| Traces | 50 GB/month, 14 days |
| **Profiles** | 50 GB/month, 14 days — **included** |
| Application Observability | 2,232 host hours |
| Dashboards | 1,000 per stack |

**10k active series is the one that bites.** OTLP histograms expand into a
series per bucket per label combination, so a single carelessly-labelled
histogram can consume the budget on its own. This is the same reason the
engineering guide forbids unbounded metric labels — the rule that protects
against a cardinality explosion locally is the rule that keeps the bill at zero
here.
