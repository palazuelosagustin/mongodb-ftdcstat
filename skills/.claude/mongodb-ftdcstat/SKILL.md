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
  [--interval N] [--avg DURATION] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME]
  [--json] [--web] [--tui] [--listen ADDR] [--verbose] [--pressure]
```

The input is the **directory**, never a single `metrics.*` file. All
`metrics.*`, `metrics.interim`, `interim*`, and exported JSON files in it are
merged as one chronological capture.

### Flag rules (the tool rejects violations)

- `--view` defaults to `summary`. `all` and `disk` are **no longer accepted**; use `summary` and `system`.
- `--interval N` (default 60) is display spacing in seconds. It does **not** aggregate; rates are still computed from adjacent raw samples.
- `--avg` averages rows into fixed buckets, `1m`–`15m` only, and cannot be combined with an explicit `--interval`. Row `datetime` is then the bucket start.
- `--from` is inclusive, `--to` is exclusive. Timestamps without a zone are UTC.
- `--json` cannot be combined with `--web` or `--tui`.
- `--verbose` expands only `repl`, `wt`, `system`, `network`. It does nothing for `summary` or `io`.
- `--pressure` (Linux PSI columns) is only valid with `--view system`.
- `--device` limits disk metrics to one device. `--view io` shows every device side by side.
- `--web` serves charts and still prints the terminal table. `--tui` serves a scrollable browser table and suppresses the terminal table. Both bind `127.0.0.1` on a random port unless `--listen` is given, and block until Ctrl+C. Run them in the background and give the user the printed URL.

## Picking a command

Start broad, then narrow to the subsystem that looks wrong.

| Question | Command |
|---|---|
| Unknown symptom, first look | `mongodb-ftdcstat DIR --avg 5m \| less -S` |
| Zoom into an incident | `mongodb-ftdcstat DIR --from 2026-06-04T19:00:00 --to 2026-06-04T20:00:00 --interval 1` |
| Replication lag / state flips | `--view repl --verbose` |
| Cache, eviction, checkpoints, tickets, history store | `--view wt --verbose` |
| CPU, memory, swap, disk latency | `--view system --verbose --pressure` |
| Which disk is slow | `--view io` (or `--view system --device sda`) |
| Op rates, latencies, global-lock queue | `--view server` |
| Connection storms, TLS/DNS slowness, rejections | `--view network --verbose` |
| Data for further processing | add `--json` |
| Visual exploration | `--web --avg 5m` or `--tui --avg 5m` |

On long captures, add `--avg` or `--from/--to` before running web or JSON
modes. The output can be very wide: when running it yourself, prefer `--json`
or a narrow focused view over dumping a full summary table into context, and
pipe large output to a file you then search.

### mongos captures

Same views, different content:

- `summary`: `router | server | network | system | io | connPool`
- `repl`: router ping and replica-set monitor metrics (`nodes`, `pingMS`, `helloOps/s`, `helloMS`, `ghaOps/s`, `ghaMS`)
- `wt`: router connection-pool / task-executor metrics (no storage engine)
- Disk columns are host-level; they show spill, logging, swap, or neighbor IO, not data-file writes by mongos.

## Reading output

- **Header first.** `Report` gives the detected process kind. `rsInfo` maps the generic `node1..nodeN` lag columns to real `host:port` members. `metricsRange` is the actual rendered span. `Parameters` shows the configured WT cache size. `network maxConn` = current + available from the first sample.
- **Restart markers** (`--- mongod restart detected: pid=... ---`) reset rate baselines; the first row after one often shows `-` for rates.
- **`0` vs `-`:** `0` is a real zero. `-` is missing path, zero denominator, counter reset, or restart boundary. In JSON, missing is `null`.
- **Formatting:** rates and percentages have 1 decimal; latencies and disk waits (seconds) have 3; raw counters are integers.
- **Tickets (`rdTkt`/`wrTkt`) are available tickets.** Low numbers mean saturation, high numbers mean idle.
- **Lag columns:** PRIMARY is `0.0`, negatives are clamped to 0, and `-` means no visible primary or no optime for that member. `majLagS` = lastWrite − majorityWrite.
- Look for sustained patterns across several rows, not single-sample spikes, and line subsystems up in time (for example, `util%`/`awaitS` rising with `wLatS` and `appEvict/s`).

Full column definitions, source FTDC paths, and formulas are in
[references/columns.md](references/columns.md). Read it when you need to
explain a specific column or verify a formula.

## Diagnostic patterns

- **Replication bottleneck:** sustained `nodeN`/`majLagS` lag, `rsState` changes, high `hbMs`, growing `applyBufCnt`/`applyBufMB`. Check `system` on the lagging member's own capture.
- **Disk bound:** high `util%`, `awaitS`/`w_awaitS`, growing `aqu-sz`, high `iowait%`, plus rising `wLatS`/`ckptMS`.
- **WT cache pressure:** `wtCache%` near or above the eviction target (~80%), `dirty%` above ~5% (dirty target) or ~20% (trigger), nonzero `appEvict/s` (application threads doing eviction stall user ops), falling tickets, long `ckptMS`, heavy `hs*` activity.
- **Connection churn:** high `totalCreated/s`, nonzero `queuedConn`/`rejConn/s`, `tlsSlow/s`/`dnsSlow/s`, `netTimeout/s`. Compare `activeConn`+`idleConn` with `maxConn`.
- **Memory / swap:** `residentMB` growth, nonzero `swapIn/s`/`swapOut/s`, `psiMemSome%`/`psiMemFull%`.
- **CPU saturation:** `user_cpu%`+`system_cpu%` near 100 (normalized across CPUs), high `psiCpuSome%`, high `ctxt/s`.

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
skill's `references/columns.md`, and the tests together. Validate with
`go test ./...` and the smoke test
`./mongodb-ftdcstat diagnostic.data --view summary --interval 43200`.
