# Diagrams

These are archived figures for the original MVP design document. They are not
the current deployment topology or complete schema. See the
[current architecture](../architecture/01-overview.md) and
[database guide](../architecture/05-database.md) for the deployed system.

Source of truth is the `.drawio` file for each diagram. All are generated from
the Mermaid sources in `src/` by `build.sh`, except `f3-inbound`, which is
produced from `src/f3-inbound.json` by the sequence-layout generator.

| File | Figure in the design document |
| --- | --- |
| `f1-topology` | 1 — deployment |
| `f2-domain` | 3 — facts, anchor, interpretation |
| `f3-inbound` | 2 — durable inbound path |
| `f4-delivery` | 5 — delivery lifecycle |
| `f5-erd` | 4 — core schema |
| `f6-telemetry` | 6 — one trace across three processes |

`svg/` feeds the PDF build. `png/` holds `-e` exports that remain editable in
draw.io.

```bash
./build.sh            # rebuild every diagram from src/
../design/build.sh    # then rebuild the PDF
```
