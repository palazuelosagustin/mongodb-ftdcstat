---
name: mongodb-ftdcstat-metrics
description: interpret mongodb-ftdcstat output from mongodb ftdc diagnostic.data captures, including terminal tables, json output, and local web api data. use when analyzing mongodb-ftdcstat metrics, explaining summary/server/wt/system/io/network/repl views and --stats/--findings reports, diagnosing mongodb or percona server performance symptoms, mapping columns to source metrics and formulas, advising which mongodb-ftdcstat command to run, or updating docs/tests that change mongodb-ftdcstat metric semantics.
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
2. Identify the selected view: `summary`, `server`, `wt`, `system`, `io`, `network`, or `repl`, and whether the output is rows or a `--stats` / `--findings` report.
3. Load `references/metric-reference.md` when column meanings, formulas, display semantics, or diagnostic hints are needed.
4. For web/API behavior, load `references/api_reference.md`.
5. Separate observed values from interpretation. State confidence and missing context when a diagnosis depends on workload, deployment, storage, cloud, container, or topology details.
6. For code changes that modify columns, formulas, flags, JSON shape, or output semantics, update the metric reference and relevant tests together.

## how to use mongodb-ftdcstat

`mongodb-ftdcstat` reads a MongoDB FTDC `diagnostic.data` directory. The input is the directory, not an individual `metrics.*` file.

Build from the repository root:

```bash
go build -o mongodb-ftdcstat ./cmd/mongodb-ftdcstat
```

General form:

```bash
mongodb-ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|io] [--avg DURATION] [--stats] [--findings] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--tui] [--listen ADDR] [--verbose] [--pressure]
```

Common command recipes:

```bash
# first look: per-column statistics plus restarts, gaps, and state changes
mongodb-ftdcstat diagnostic.data --stats --findings

# broad triage, one row per FTDC sample
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

# every disk side by side
mongodb-ftdcstat diagnostic.data --view io

# ticket queues (7.0+), query efficiency, write conflicts, transactions
mongodb-ftdcstat diagnostic.data --view server --verbose

# replica-set lag and state
mongodb-ftdcstat diagnostic.data --view repl --verbose

# connection activity and connection-establishment symptoms
mongodb-ftdcstat diagnostic.data --view network --verbose
```

Flag rules to preserve in answers:

- `--view summary` is the default and is intended for horizontal scrolling with `less -S`.
- `--view all` and `--view disk` are no longer accepted; use `summary` and `system`.
- Every FTDC sample is one row (normally one per second). `--interval` is no longer accepted.
- `--avg DURATION` averages derived rows into fixed buckets; valid bucket sizes are `1m` through `15m`.
- `--stats` prints per-column n, null%, min, avg, p50/p95/p99, max, and max time instead of rows; `--findings` prints a timeline of restarts, gaps, and `rsState` changes (no thresholds). They can be combined and cannot be combined with `--avg`, `--web`, or `--tui`.
- `--from` is inclusive; `--to` is exclusive.
- `--json` cannot be combined with `--web` or `--tui`.
- `--verbose` expands only focused views: `server`, `repl`, `wt`, `system`, and `network`; it is rejected for `summary` and `io`.
- `--pressure` is only valid with `--view system`; with `--verbose`, PSI columns are appended as a separate `pressure` section.

When advising a command, pick the narrowest command that answers the question. Use `--stats --findings` first for unknown symptoms, then focused views for follow-up: `wt` for cache/eviction/checkpoint/tickets/history store, `system` for CPU/disk/memory/swap/PSI, `io` to compare disks, `repl` for lag/state, `network` for connection behavior, and `server` for operation rates, latency, and queues.

## working order when you run it yourself

Every FTDC sample is a row, so a full capture is about 86,400 rows per day. Never print all rows into context. Work in this order:

1. **`--stats --findings`** (text) for the whole capture. It is a few KB, it reads every sample, and `max at` plus the event times give the windows to look at. Then `--view X --stats` for the subsystem that moved.
2. **`--stats` on a narrower window or view** to compare periods, for example the p99 of `w_awaitS` before and during an incident: `--view io --stats --from ... --to ...`.
3. **Text rows for a short window** (minutes, not hours) to read the sequence of events: `--view wt --from ... --to ...`.
4. **`--json` piped through `jq`** for questions about specific values: when, how often, how long, or conditions across columns. Read only the `jq` output, never the raw JSON. For the same rows, JSON costs about 3x the tokens of the text table (every value repeats its key). Use it also when you need exact values: the text table rounds.

```bash
F="mongodb-ftdcstat DIR --view wt --json --from 2026-06-11T19:00:00 --to 2026-06-11T23:00:00"
# seconds where application threads evicted pages
$F | jq -r '.rows[] | select((.wiredTiger["appEvict/s"] // 0) > 0) | [.time, .wiredTiger["appEvict/s"]] | @tsv'
# how many seconds the cache was at or above 80%
$F | jq '[.rows[] | select((.wiredTiger["wtCache%"] // 0) >= 80)] | length'
```

For `--web`/`--tui` on long captures, add `--avg` or `--from/--to` first.

## interpreting pasted output

When the user pastes output:

- Use the header tables first: `Report` has the process kind, `Samples`, and `From`/`To` (the rendered span); `Replica Set` maps `node1..nodeN` labels to real members; `Host` and `Build` give hardware and version. Process markers before rows indicate restarts. In JSON the same facts are in `metadata` and `rsInfo`.
- Treat `0` as present and zero. Treat `-` as missing, unavailable, undefined, zero denominator, counter reset, or not computable.
- In JSON, missing/unavailable values appear as `null`.
- Unless `--avg` is present, every FTDC sample is a row and rates are calculated from adjacent raw samples. Use `--avg` or `--from/--to` on long captures.
- Map generic replica-set node columns through the `Replica Set` table (`rsInfo` in JSON) before naming a host.
- `rdTkt`/`wrTkt` are available tickets. Before 7.0, low values mean saturation; from 7.0 tickets are adjusted dynamically, so a low value alone does not.
- Avoid overdiagnosis from one spike; prefer sustained patterns across multiple rows.

## output style

For metric explanations, use this structure when helpful:

```text
view: <detected view>
time range: <Report From/To or row span if available>
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
