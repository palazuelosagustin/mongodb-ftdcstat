---
name: mongodb-ftdcstat
description: Run and interpret mongodb-ftdcstat, a Go CLI that turns a MongoDB / Percona Server for MongoDB FTDC diagnostic.data directory into terminal tables, JSON, or a local web dashboard. Use when the user has a diagnostic.data directory (mongod or mongos) and wants a performance timeline, asks which mongodb-ftdcstat view or flags to use, pastes mongodb-ftdcstat output to explain, or wants to diagnose replication lag, WiredTiger cache/eviction/checkpoint pressure, ticket exhaustion, disk latency, CPU/memory/swap/PSI pressure, connection churn, or mongos router connection-pool health from FTDC. Also use when changing mongodb-ftdcstat code, columns, formulas, or docs. Not for slow-query logs (use mongodb-logstat) or for sanitizing customer data (use mongodb-log-obfuscator-go).
---

# mongodb-ftdcstat

Reads a MongoDB FTDC `diagnostic.data` directory, auto-detects `mongod` vs
`mongos` from `serverStatus.process`, and prints derived metrics as wide
terminal tables, `--json`, or a local browser UI (`--web` / `--tui`).

## Privacy first

Customer FTDC contains hostnames, IPs, replica-set names, and config values.
For support cases, sanitize with `mongodb-log-obfuscator-go` first and run this
tool on the obfuscated `diagnostic.data` copy, unless the user explicitly says
the data is not sensitive. This tool is not an obfuscator: it prints whatever
identifiers are in the capture (`rsInfo`, `hostInfo`, `getCmdLineOpts`).

## Locating the binary

1. `command -v mongodb-ftdcstat`
2. Otherwise the repo checkout, e.g. `~/bin/mongodb-ftdcstat/mongodb-ftdcstat`
3. Otherwise build from the repo root: `go build -o mongodb-ftdcstat ./cmd/mongodb-ftdcstat`

If you changed the source in this session, rebuild before running.

## Usage

```text
mongodb-ftdcstat <diagnostic.data-dir> [--view summary|server|wt|system|io|network|repl]
  [--avg DURATION] [--stats] [--findings] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME]
  [--json] [--web] [--tui] [--listen ADDR] [--verbose] [--pressure]
```

The input is the **directory**, never a single `metrics.*` file. All
`metrics.*`, `metrics.interim`, `interim*`, and exported JSON files in it are
merged as one chronological capture.

### Flag rules (the tool rejects violations)

- `--view` defaults to `summary`. `all` and `disk` are **no longer accepted**; use `summary` and `system`.
- Every FTDC sample is one row (normally one per second). `--interval` is **no longer accepted**; use `--avg` or `--from/--to` to reduce output.
- `--avg` averages rows into fixed buckets, `1m`–`15m` only. Row `datetime` is then the bucket start.
- `--stats` prints per-column n, null%, min, avg, p50/p95/p99, max, and max time for the view instead of rows. Percentiles are within 1%.
- `--findings` prints a timeline of facts: first process, restarts (pid, start), gaps without samples, and `rsState` changes. No thresholds. It does not depend on `--view`.
- `--stats` and `--findings` can be combined (one pass). They work with `--view`, `--verbose`, `--pressure`, `--device`, `--from/--to`, `--json`, and are rejected with `--avg`, `--web`, `--tui`. `--report` is no longer accepted.
- `--from` is inclusive, `--to` is exclusive. Timestamps without a zone are UTC.
- `--json` cannot be combined with `--web` or `--tui`.
- `--listen` is only valid with `--web` or `--tui`.
- Every flag also accepts the `--flag=value` form.
- `--verbose` is only valid with `server`, `repl`, `wt`, `system`, `network`; it is rejected for `summary` and `io`.
- `--pressure` (Linux PSI columns) is only valid with `--view system`.
- `--device` limits disk metrics to one device. `--view io` shows every device side by side.
- `--web` serves charts and still prints the terminal table. `--tui` serves a scrollable browser table and suppresses the terminal table. Both bind `127.0.0.1` on a random port unless `--listen` is given, and block until Ctrl+C. Run them in the background and give the user the printed URL (`webUI` header section).

## Picking a command

Start broad, then narrow to the subsystem that looks wrong.

| Question | Command |
|---|---|
| Unknown symptom, first look | `mongodb-ftdcstat DIR --stats --findings` (then `--view X --stats` per subsystem) |
| See the timeline | `mongodb-ftdcstat DIR --avg 5m \| less -S` |
| Zoom into an incident | `mongodb-ftdcstat DIR --from 2026-06-04T19:00:00 --to 2026-06-04T20:00:00` |
| Replication lag / state flips | `--view repl --verbose` |
| Cache, eviction, checkpoints, tickets, history store | `--view wt --verbose` |
| CPU, steal, memory, swap, disk latency, dbPath disk space | `--view system --verbose --pressure` |
| Which disk is slow | `--view io` (or `--view system --device sda`) |
| Op rates, latencies, global-lock queue | `--view server` |
| Ticket queues (7.0+), inefficient queries, write conflicts, transactions | `--view server --verbose` |
| Connection storms, TLS/DNS slowness, rejections | `--view network --verbose` |
| When / how often / how long, exact values | `--json` piped through `jq` (see below) |
| Visual exploration | `--web --avg 5m` or `--tui --avg 5m` |

### Working order when you run it yourself

Every FTDC sample is a row, so a full capture is about 86,400 rows per day.
Never print all rows into context. Work in this order:

1. **`--stats --findings`** (text) for the whole capture. It is a few KB, it
   reads every sample, and `max at` plus the event times give the windows to
   look at. Then `--view X --stats` for the subsystem that moved.
2. **`--stats` on a narrower window or view** to compare periods, for example
   the p99 of `w_awaitS` before and during an incident:
   `--view io --stats --from ... --to ...`.
3. **Text rows for a short window** (minutes, not hours) to read the sequence
   of events: `--view wt --from ... --to ...`.
4. **`--json` piped through `jq`** for questions about specific values: when,
   how often, how long, or conditions across columns. Read only the `jq`
   output, never the raw JSON. For the same rows, JSON costs about 3x the
   tokens of the text table (every value repeats its key). Use it also when
   you need exact values: the text table rounds.

```bash
F="mongodb-ftdcstat DIR --view wt --json --from 2026-06-11T19:00:00 --to 2026-06-11T23:00:00"
# seconds where application threads evicted pages
$F | jq -r '.rows[] | select((.wiredTiger["appEvict/s"] // 0) > 0) | [.time, .wiredTiger["appEvict/s"]] | @tsv'
# how many seconds the cache was at or above 80%
$F | jq '[.rows[] | select((.wiredTiger["wtCache%"] // 0) >= 80)] | length'
```

More recipes, including lag per real host and filtering findings events, are
in [references/json-api.md](references/json-api.md).

For `--web`/`--tui` on long captures, add `--avg` or `--from/--to` first.

### mongos captures

Same views, different content:

- `summary`: `router | server | network | system | io | connPool`
- `repl`: router ping and replica-set monitor metrics (`nodes`, `pingMS`, `helloOps/s`, `helloMS`, `ghaOps/s`, `ghaMS`)
- `wt`: router connection-pool / task-executor metrics (no storage engine)
- Disk columns are host-level; they show spill, logging, swap, or neighbor IO, not data-file writes by mongos.

## Reading output

- **Header first.** It is a set of tables. `Report` gives the process kind, `Samples`, and `From`/`To` (the actual rendered span). `Host` has CPU cores, memory, and `Max connections` (current + available from the first sample). `Build` has the version. `Replica Set` maps the generic `node1..nodeN` lag columns to real `host:port` members. `Parameters` shows the configured WT cache size. The same facts are in `metadata` and `rsInfo` in JSON.
- **Restart markers** (`--- mongod restart detected: pid=... ---`) reset rate baselines; the first row after one often shows `-` for rates.
- **`0` vs `-`:** `0` is a real zero. `-` is missing path, zero denominator, counter reset, or restart boundary. In JSON, missing is `null`.
- **Formatting:** rates and percentages have 1 decimal; latencies and disk waits (seconds) have 3; raw counters are integers.
- **Tickets (`rdTkt`/`wrTkt`) are available tickets.** Before 7.0, low numbers mean saturation. From 7.0 the ticket count is adjusted dynamically, so a low value alone does not mean saturation.
- **Lag columns:** PRIMARY is `0.0`, negatives are clamped to 0, and `-` means no visible primary or no optime for that member. `majLagS` = lastWrite − majorityWrite.
- Look for sustained patterns across several rows, not single-sample spikes, and line subsystems up in time (for example, `util%`/`awaitS` rising with `wLatS` and `appEvict/s`).

Full column definitions, source FTDC paths, and formulas are in
[references/columns.md](references/columns.md). Read it when you need to
explain a specific column or verify a formula.

For `--json` structure, `jq` recipes, and the `--web`/`--tui` HTTP endpoints
(`/api/metadata`, `/api/data`, `/api/table`), read
[references/json-api.md](references/json-api.md).

## Diagnostic patterns

- **Replication bottleneck:** sustained `nodeN`/`majLagS` lag, `rsState` changes, high `hbMs`, growing `applyBufCnt`/`applyBufMB`. Check `system` on the lagging member's own capture.
- **Disk bound:** high `util%`, `awaitS`/`w_awaitS`, growing `aqu-sz`, high `iowait%`, plus rising `wLatS`/`ckptMS`.
- **WT cache pressure:** `wtCache%` near or above the eviction target (~80%), `dirty%` above ~5% (dirty target) or ~20% (trigger), nonzero `appEvict/s` (application threads doing eviction stall user ops), long `ckptMS`, heavy `hs*` activity. Falling tickets count only before 7.0 (see above).
- **Connection churn:** high `totalCreated/s`, nonzero `queuedConn`/`rejConn/s`, `tlsSlow/s`/`dnsSlow/s`, `netTimeout/s`. Compare `activeConn`+`idleConn` with `maxConn`.
- **Memory / swap:** `residentMB` growth, falling `memAvailMB`, nonzero `swapIn/s`/`swapOut/s`, `psiMemSome%`/`psiMemFull%`. A large `heapMB - allocMB` gap is memory tcmalloc holds but mongod does not use; it explains `residentMB` well above the WT cache size.
- **Disk space:** low or falling `dbFree%` / `dbFreeGB` (`--view system --verbose`). It is the filesystem holding `storage.dbPath`; check it before blaming IO latency on a nearly full disk.
- **Ticket queueing (7.0+):** nonzero `rdQ`/`wrQ` with rising `rdQMS`/`wrQMS` means operations wait for admission; line it up with `rLatS`/`wLatS` and WT cache pressure.
- **Inefficient queries:** `keys/ret` or `docs/ret` in the hundreds or more, nonzero `collScans/s`; a high `docs/ret` with `keys/ret` near 0 is a collection scan. Find the queries with `mongodb-logstat` for that window.
- **Write contention:** sustained `wrConfl/s`, rising `txnAbort/s`.
- **CPU saturation:** `user_cpu%`+`system_cpu%` near 100 (normalized across CPUs), high `psiCpuSome%`, high `ctxt/s`, `procsRun` well above the core count. `steal%` above a few percent means the hypervisor is taking CPU from the VM.

## Answer format

When explaining output, separate what was observed from what it means:

```text
capture: <process kind, metricsRange, node mapping if relevant>
observations: <column = value / trend, with timestamps>
interpretation: <likely cause, with confidence>
next checks: <exact mongodb-ftdcstat command(s) or other evidence needed>
```

Say what context is missing (workload, storage type, cloud/container limits,
topology) instead of guessing. FTDC shows that something happened, not which
queries caused it; for that, correlate the time window with the slow-query log
using `mongodb-logstat --report analysis` (its `hotWindows` align with FTDC).

## Changing the tool

Columns live in `internal/render/metrics.go` (registry and per-view column
lists), derivations in `internal/derive/derive.go`, and FTDC path selection in
`internal/derive/paths.go`. If you change a column, formula, flag, or JSON
shape, update `README.md` (and `readme_test.go` enforces parts of it), this
skill's `references/columns.md` (and `references/json-api.md` for JSON/API
changes), and the tests together. Web routes live in `internal/webui/server.go`. Validate with
`go test ./...` and the smoke test
`./mongodb-ftdcstat diagnostic.data --view summary --avg 15m`.
