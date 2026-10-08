---
name: mongodb-ftdcstat-metrics
description: interpret mongodb-ftdcstat output from mongodb ftdc diagnostic.data captures, including terminal tables, json output, and local web api data. use when analyzing mongodb-ftdcstat metrics, explaining summary/server/wt/system/network/repl views, diagnosing mongodb or percona server performance symptoms, mapping columns to source metrics and formulas, advising which mongodb-ftdcstat command to run, or updating docs/tests that change mongodb-ftdcstat metric semantics.
---

# mongodb-ftdcstat metrics

## overview

Use this skill to read, explain, and troubleshoot `mongodb-ftdcstat` output from MongoDB or Percona Server FTDC `diagnostic.data` captures. Prefer the bundled reference over re-reading project source when interpreting metric columns, formulas, output formatting, command flags, or common performance symptoms.

## workflow

1. Determine what the user provided:
   - pasted terminal table output
   - `--json` output
   - local web API data from `/api/metadata` or `/api/data`
   - a request for which `mongodb-ftdcstat` command to run
   - a code/docs/test change that affects metric semantics
2. Identify the selected view: `summary`, `server`, `wt`, `system`, `network`, `repl`, or aliases `all`/`disk`.
3. Load `references/metric-reference.md` when column meanings, formulas, display semantics, or diagnostic hints are needed.
4. For web/API behavior, load `references/api_reference.md`.
5. Separate observed values from interpretation. State confidence and missing context when a diagnosis depends on workload, deployment, storage, cloud, container, or topology details.
6. For code changes that modify columns, formulas, flags, JSON shape, or output semantics, update the metric reference and relevant tests together.

## how to use mongodb-ftdcstat

`mongodb-ftdcstat` reads a MongoDB FTDC `diagnostic.data` directory. The input is the directory, not an individual `metrics.*` file.

Build from the repository root:

```bash
go build -o mongodb-ftdcstat ./cmd/ftdcstat
```

General form:

```bash
mongodb-ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--avg DURATION] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--listen ADDR] [--verbose] [--pressure]
```

Common command recipes:

```bash
# broad triage, one compact row per display interval
mongodb-ftdcstat diagnostic.data --view summary | less -S

# broad triage with fewer rows for long captures
mongodb-ftdcstat diagnostic.data --view summary --avg 5m | less -S

# restrict analysis to a specific time window
mongodb-ftdcstat diagnostic.data --view summary --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"

# machine-readable output for follow-up analysis
mongodb-ftdcstat diagnostic.data --view summary --json

# local plotting UI; still prints terminal output
mongodb-ftdcstat diagnostic.data --web --view summary --avg 5m

# WiredTiger cache, checkpoint, eviction, tickets, and history store details
mongodb-ftdcstat diagnostic.data --view wt --verbose

# CPU, memory, disk, swap, and Linux PSI pressure
mongodb-ftdcstat diagnostic.data --view system --verbose --pressure

# one disk only when many devices are present
mongodb-ftdcstat diagnostic.data --view system --device sda

# replica-set lag and state
mongodb-ftdcstat diagnostic.data --view repl --verbose

# connection activity and connection-establishment symptoms
mongodb-ftdcstat diagnostic.data --view network --verbose
```

Flag rules to preserve in answers:

- `--view summary` is the default and is intended for horizontal scrolling with `less -S`.
- `--view all` is a compatibility alias for `summary`.
- `--view disk` is a compatibility alias for `system`.
- `--interval N` controls display spacing in seconds; it does not aggregate metrics.
- `--avg DURATION` averages derived rows into fixed buckets; valid bucket sizes are `1m` through `15m`; it cannot be combined with explicit `--interval`.
- `--from` is inclusive; `--to` is exclusive.
- `--json` cannot be combined with `--web`.
- `--verbose` expands only focused views: `repl`, `wt`, `system`, and `network`; it does not expand `summary`.
- `--pressure` is only valid with `--view system`; with `--verbose`, PSI columns are appended as a separate `pressure` section.

When advising a command, pick the narrowest command that answers the question. Use `summary` first for unknown symptoms, then focused views for follow-up: `wt` for cache/eviction/checkpoint/tickets/history store, `system` for CPU/disk/memory/swap/PSI, `repl` for lag/state, `network` for connection behavior, and `server` for operation rates, latency, and queues.

## interpreting pasted output

When the user pastes output:

- Use header sections first: `rsInfo` maps `node1..nodeN` labels to real replica-set members, `metricsRange` bounds the rendered rows, and process markers indicate restarts.
- Treat `0` as present and zero. Treat `-` as missing, unavailable, undefined, zero denominator, counter reset, or not computable.
- In JSON, missing/unavailable values appear as `null`.
- Do not confuse display interval with aggregation. Unless `--avg` is present, rows are sampled at display spacing and rates are calculated from adjacent raw samples.
- Map generic replica-set node columns through `rsInfo` before naming a host.
- Avoid overdiagnosis from one spike; prefer sustained patterns across multiple rows.

## output style

For metric explanations, use this structure when helpful:

```text
view: <detected view>
time range: <metricsRange or row span if available>
key observations:
- <value/pattern and where it appears>
interpretation:
- <what it likely means>
checks to confirm:
- <next mongodb-ftdcstat command or specific metric/view>
```

For diagnosis, group findings by subsystem: replication, server, network, system, WiredTiger. Mention missing context explicitly rather than guessing.

## bundled references

- `references/metric-reference.md`: views, flags, columns, formulas, formatting, missing-value semantics, and diagnostic hints.
- `references/api_reference.md`: local web mode and JSON/API behavior.
