# mongodb-ftdcstat

`mongodb-ftdcstat` reads a MongoDB `diagnostic.data` directory, auto-detects
`mongod` versus `mongos`, and prints FTDC metrics as terminal-friendly tables or
serves a local plotting UI.

## Build

```bash
go build -o mongodb-ftdcstat ./cmd/mongodb-ftdcstat
```

## Usage

```bash
mongodb-ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|io] [--avg DURATION] [--stats] [--findings] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--tui] [--listen ADDR] [--verbose] [--pressure]
```

The input is a directory, not a single FTDC file. The tool discovers
`metrics.*`, `metrics.interim`, `interim*`, and exported JSON files, then treats
them as one chronological capture.

Process type detection comes from FTDC `serverStatus.process`. When it is `mongod`, the tool renders the existing mongod report layout. When it is `mongos`, the tool renders the router-specific mongos layout. Top-level `router` metrics alone do not classify a capture as mongos, because shard data nodes can include router-related metrics. If `serverStatus.process` is missing, output falls back to the mongod-style report.

## Options

### `<path-to-diagnostic-data-directory>`

Required. Path to a MongoDB FTDC diagnostic data directory.

### `--view server|wt|system|network|repl|summary|io`

Default: `summary`.

Views:

```text
server   MongoDB serverStatus counters, latency, and queues
wt       `mongod`: WiredTiger cache, eviction, checkpoint, and ticket metrics
         `mongos`: router connection-pool and executor metrics
system   CPU, memory, and disk metrics
io       Per-device disk metrics
network  Connection activity and network-establishment diagnostics
repl     `mongod`: replica-set lag and replication state
         `mongos`: router ping and replica-set monitor metrics
summary  `mongod`: replication, server, network, system, io, and WiredTiger columns
         `mongos`: router, server, network, system, io, and connection-pool columns
```

`--view disk` is no longer supported; use `--view system`.
`--view all` is no longer supported; use `--view summary`.

`--view summary` is intended for horizontal scrolling:

```bash
mongodb-ftdcstat diagnostic.data --view summary | less -S
```

It prints one wide table, one row per display interval, and repeats the compact
section-label row plus the column header every 50 data rows. The wide table uses
`|` separators after `datetime` and between logical metric groups. It avoids the
old full-width dashed banner lines. The `--view summary` section order is:

```text
mongod: replication | server | network | system | io | wiredTiger
mongos: router | server | network | system | io | connPool
```

### Row spacing

Every FTDC sample becomes one row. With the default MongoDB
`diagnosticDataCollectionPeriodMillis=1000`, that is about one row per second.
Rates are calculated from adjacent raw samples.

`--interval` is no longer supported and is rejected with an error. Use `--avg`
to group rows into time buckets, or `--from` / `--to` to narrow the range.

### `--avg DURATION`

`--avg` averages derived rows into fixed time buckets for both the terminal
report and the local web UI.

```bash
mongodb-ftdcstat diagnostic.data --view summary --avg 5m
mongodb-ftdcstat diagnostic.data --web --view summary --avg 5m
```

Valid bucket sizes are `1m` through `15m`. `--avg` rejects smaller or larger
durations.

When enabled, `mongodb-ftdcstat` prints:

```text
Averaging: 5m buckets; datetime is bucket start; values are averaged per bucket.
```

right before the metrics table. Empty buckets are skipped.

### `--stats` and `--findings`

Both print a short report instead of one row per sample. They can be used
alone or together; together they still read the capture only once.

#### `--stats`

Prints per-column statistics for the selected view instead of one row per
sample. It is a quick first look at a long capture: which columns ever moved,
how high they went, and when.

```bash
mongodb-ftdcstat diagnostic.data --stats
mongodb-ftdcstat diagnostic.data --stats --view wt --verbose
mongodb-ftdcstat diagnostic.data --stats --view io --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"
mongodb-ftdcstat diagnostic.data --stats --findings --json
```

The normal header tables are printed first; the `Report` table shows
`Mode  stats` (or `stats, findings`). Then one line per numeric column of the view:

```text
section     column      n        null%  min   avg   p50   p95   p99   max     max at
wiredTiger  wtCache%    1150328  0.0    0.0   51.7  53.0  64.7  73.0  85.7    2026-06-11T20:20:12Z
wiredTiger  appEvict/s  1150322  0.0    0.0   0.0   0.0   0.0   0.0   396.0   2026-06-11T20:20:32Z
```

```text
n        rows where the column has a value
null%    rows where it is missing (-), for example rates after a restart or
         latencies with no operations
min avg  exact
p50 p95 p99
         estimated with a streaming sketch, within 1% of a real value at
         that rank
max      exact
max at   time of the first row that reached max
```

Values use the same formatting as the table, except `avg` keeps one decimal
for integer columns. Percentages that are not exactly 0 or 100 never print as
`0.0` or `100.0`: they print as `<0.1` or `>99.9`, so a column with a handful of
values is not shown as always empty. JSON keeps the exact values.

Text columns such as `rsState` are summarized below the table as the share of rows per value plus the number of changes:

```text
replication rsState: PRIMARY 99.2%, SECONDARY 0.8%; 9 changes
```

The line above the table counts rows, restarts, and capture gaps (rate
baseline resets).

Statistics are always computed from every sample, so a short spike is not
averaged away. The capture is read once and rows are not kept in memory.

#### `--findings`

A timeline of what happened in the capture. It reports facts only, with no
thresholds or severity:

```text
Findings: 15 events

2026-06-02T12:40:39Z  process  mongod pid=524587 start=2026-05-20T22:37:07Z
2026-06-04T23:00:18Z  rsState  PRIMARY -> SECONDARY
2026-06-04T23:25:27Z  gap      24m55s without samples, rate baseline reset
2026-06-04T23:25:27Z  restart  mongod pid=656311 start=2026-06-04T23:25:26Z
2026-06-04T23:25:38Z  rsState  SECONDARY -> PRIMARY
```

```text
process  first process in the range: pid and start time (start = sample time - uptime)
restart  a new process: pid changed or uptime went down
gap      time without samples longer than 60s; rates restart after it
rsState  the node's replica-set state changed (mongod only)
```

Events do not depend on `--view`. Times are the first row that shows the
event. Use them with `--from` / `--to` to zoom in.

#### Rules

```text
--stats and --findings work with --view, --verbose, --pressure, --device, --from, --to, and --json
--stats and --findings cannot be combined with --avg, --web, or --tui
--report is no longer supported; it is rejected with a pointer to --stats and --findings
```

With `--json`, one document holds `reports` (`["stats"]`, `["findings"]`, or
both), `view`, `metadata`, `rsInfo`, `warnings`, `range` (`start`, `end`), and
the requested reports. `--stats` adds `percentileRelativeAccuracy` and `stats`:

```text
stats.rows, stats.restarts, stats.gaps
stats.columns[]  section, subsection (io device in --view io), column, n, nullPct,
                 min, avg, p50, p95, p99, max, maxAt (null when n is 0)
stats.states[]   section, column, values[] {value, rows, pct}, changes
```

`--findings` adds `findings.events[]`, each with `time` and `type`, plus only
the fields for that type:

```text
process, restart  process (mongod or mongos), pid, start
gap               seconds
rsState           from, to
```

JSON values are unrounded.

### `--device DEVICE`

Filters disk-derived fields to a single device.

```bash
mongodb-ftdcstat diagnostic.data --view io --device sda
```

Without `--device`, `--view system` and `--view summary` use the existing disk rollup.
With `--view io`, devices are rendered independently in stable alphabetical order.

### `--from ISO_TIME` and `--to ISO_TIME`

Filters samples to a time range.

```bash
mongodb-ftdcstat diagnostic.data --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"
```

Rules:

```text
--from  inclusive
--to    exclusive
```

Timestamps with a timezone are parsed as absolute times. Timestamps without a
timezone are interpreted as UTC. Rotated metric filenames are
used to skip files that cannot overlap the requested range when that can be
determined; files with unknown ranges are parsed conservatively.

### `--json`

Prints JSON instead of terminal tables. JSON includes selected metadata,
warnings, selected view, derived rows, and an `rsInfo.members` mapping from
generic replica-set node labels to real member names. Each row includes grouped
section objects such as `replication` and `server`; `replication.lagS` contains
the per-node lag values, `replication.majLagS` contains the majority commit lag,
and unavailable lag values are `null`.

`metadata` is a compact summary of the capture, with the same facts as the CLI
header tables. It is not the raw FTDC metadata documents (`buildInfo`,
`hostInfo`, `getParameter`, ...), which are tens of KB of mostly defaults and
are not included in any output:

```text
process      mongod or mongos
host         hostname, os, kernel, arch, cpuCores, memoryMB, maxConnections
build        version, gitVersion, modules, storageEngine, allocator, openssl,
             perconaFeatures (lists are JSON arrays)
cmdLineOpts  parsed startup options as flat dotted keys with their JSON types,
             for example "storage.wiredTiger.engineConfig.cacheSizeGB": 0.5;
             setParameter is listed under parameters instead
parameters   wtCache plus explicitly configured setParameter values
```

Missing fields are left out. The same summary is used by `--stats` /
`--findings` JSON and by the web `/api/metadata` endpoint.

### `--web`

`--web` starts the local dashboard UI and still prints the normal terminal table.

```bash
mongodb-ftdcstat diagnostic.data --web
mongodb-ftdcstat diagnostic.data --web --view summary --avg 5m
mongodb-ftdcstat diagnostic.data --web --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"
```

### `--tui`

`--tui` starts a browser-based table UI with a fixed `datetime` column. The CLI
still prints the report header, but it does not print the metrics table because
those metrics are already available in the Web TUI.

```bash
mongodb-ftdcstat diagnostic.data --tui
mongodb-ftdcstat diagnostic.data --tui --view summary --avg 5m
mongodb-ftdcstat diagnostic.data --web --tui --view summary
```

Behavior for `--web` and `--tui`:

```text
starts one local HTTP server for either mode or both together
binds to 127.0.0.1 on a random available port by default
prints CLI header tables in normal CLI, --web, and --tui modes
prints the CLI metrics table in normal CLI and --web modes
suppresses the CLI metrics table whenever --tui is enabled
prints `Web UI: ...` when --web is enabled
prints `Web TUI: ...` when --tui is enabled
prints `HTTP server is running. Press Ctrl+C to stop.` after the report
reuses the existing FTDC parsing and derived-row pipeline
keeps --view, --from, --to, --verbose, and --pressure semantics
```

Example stdout with `--web --tui`:

```text
Web UI:  http://127.0.0.1:49231/
Web TUI: http://127.0.0.1:49231/tui

Report
+----------------+--------------------------+
| Field          | Value                    |
+----------------+--------------------------+
| Path           | diagnostic.data          |
| View           | summary                  |
| From           | 2026-06-04T19:00:00Z     |
| To             | 2026-06-04T20:00:00Z     |
| Process        | mongod                   |
+----------------+--------------------------+

...

HTTP server is running. Press Ctrl+C to stop.
```

For large captures, prefer combining `--web` or `--tui` with `--avg` or with
`--from` and `--to` to keep browser rendering responsive.

When the plotted dataset is large, the tool warns:

```text
warning: Large capture detected. Consider using --avg 5m or --from/--to for better browser performance.
```

`--json` cannot be combined with `--web` or `--tui`.

The local HTTP server exposes:

```text
GET /              -> embedded dashboard index.html
GET /app.js        -> dashboard JavaScript
GET /style.css     -> dashboard CSS
GET /tui           -> embedded Web TUI page
GET /tui.js        -> Web TUI JavaScript
GET /tui.css       -> Web TUI CSS
GET /api/metadata  -> capture/header metadata
GET /api/data      -> selected derived rows or chart data
GET /api/table     -> flattened table rows for the Web TUI
```

`/api/metadata` still includes `headerText` for compatibility, but neither the
Web UI nor the Web TUI renders that CLI-style header block in the browser.
Header metadata is now terminal-only.

The dashboard charts are grouped by the same logical sections as the selected
view. For server metrics, the web UI splits charts into `server / Commands` and
`server / Latency`, plus `server / Admission`, `server / Query efficiency`, and
`server / Transactions` with `--verbose`. For system metrics, the web UI splits
charts into `system / CPU`, `system / Processes`, `system / Memory`,
`system / Disks`, `system / Disk space`, and `system / PSI` when those metric
groups are present in the loaded data. For `--view io`, the web UI
renders one chart section per metric with one line per device. For WiredTiger metrics, the
web UI splits charts into `wiredTiger / Tickets`,
`wiredTiger / Per-second rates`, `wiredTiger / Checkpoint time`,
`wiredTiger / Percentages`, `wiredTiger / MiB`, `wiredTiger / App thread time`,
and `wiredTiger / Journal` when those metric groups are present in the loaded
data. Hovering a section chart shows a Grafana-like
tooltip with the exact UTC timestamp and the visible series values at that
timestamp.

The Web TUI renders the same report as a horizontally scrollable table with a
sticky header row, sticky `datetime` column, grouped device headers for `--view io`, and keyboard navigation using
`h`, `j`, `k`, `l`, `g`, `G`, and the arrow keys.

### `--listen ADDR`

Overrides the default local bind address used by `--web` or `--tui`:

```bash
mongodb-ftdcstat diagnostic.data --web --listen 127.0.0.1:8080
mongodb-ftdcstat diagnostic.data --tui --listen 127.0.0.1:8080
```

### `--verbose`

`--verbose` expands columns for focused views only. It applies to
`--view server`, `--view repl`, `--view wt`, `--view system`, and
`--view network`. It is rejected for `--view summary` and `--view io`, which
always print the compact rollup for the detected process type.

When used with `--view server`, `--verbose` adds admission-queue, query
efficiency, write-conflict, and transaction columns after the default server
columns:

```text
qTot ins/s qry/s upd/s del/s getm/s cmd/s rLatS wLatS cLatS rdQ wrQ rdQMS wrQMS wrConfl/s collScans/s keys/ret docs/ret txnActive txnCommit/s txnAbort/s
```

When used with `--view repl`, `--verbose` adds replication apply/buffer,
fetch, oplog size, and flow control metrics after `majLagS`:

```text
lagS node1 node2 ... nodeN majLagS hbMs applyOps/s applyBufCnt applyBufMB replNetMB/s oplogMB oplogUsed% flowLagged flowRate flowWaitMS/s
```

For `mongos`, `--view repl` remains the focused router-health view for CLI
compatibility and prints router ping plus replica-set monitor metrics.

When used with `--view wt`, `--verbose` expands the WiredTiger columns with
cache, eviction, checkpoint, ticket, history store, application-thread time,
and journal diagnostics:

```text
wtCache% dirty% cacheMB dirtyMB updatesMB wtRdMB/s wtWrMB/s evict/s appEvict/s evictWalks/s evictBusy/s ckptMS ckptPages/s rdTkt wrTkt hsInsert/s hsRead/s hsWriteMB/s appEvictMS/s appReadMS/s logMB/s logSyncMS
```

For `mongos`, `--view wt` remains the focused pool/executor view for CLI
compatibility and expands router connection-pool metrics instead of storage
metrics.

When used with `--view system`, `--verbose` adds disk throughput, context
switches, swap activity, CPU steal, process counts, host memory, the tcmalloc
heap, and free space on the dbPath filesystem after the default system columns:

```text
r/s w/s rkB/s wkB/s awaitS r_awaitS w_awaitS aqu-sz util% user_cpu% system_cpu% iowait% residentMB virtualMB ctxt/s swapIn/s swapOut/s steal% procsRun procsBlk memAvailMB cachedMB osDirtyMB heapMB allocMB dbFree% dbFreeGB
```

When used with `--view network`, `--verbose` appends queued and connection-establishment
symptom counters after the default network columns:

```text
activeConn idleConn totalCreated/s queuedConn rejConn/s dnsSlow/s tlsSlow/s netTimeout/s
```

Verbose replication metrics:

```text
hbMs         average replSetGetStatus.members[].pingMs across members, milliseconds
applyOps/s   delta(serverStatus.metrics.repl.apply.ops) / elapsed seconds
applyBufCnt  serverStatus.metrics.repl.buffer.apply.count
applyBufMB   serverStatus.metrics.repl.buffer.apply.sizeBytes / 1024 / 1024
```

These correspond to the `metrics.repl.*` fields returned by `serverStatus` in
mongod. FTDC stores them under the `serverStatus.metrics.repl.*` path prefix.

`applyOps/s` uses the same rate baseline reset rules as `ins/s`, `qry/s`, and other
counter-derived rates. Normal FTDC file rotations preserve continuity; large
gaps, process restarts, and counter resets suppress the rate.

Verbose and pressure columns read their extra FTDC paths only when the flag
is given, so the default views read less data.

### `--pressure`

`--pressure` is only supported for `--view system`. It appends Linux PSI columns
in a separate `pressure` section after the system columns:

```text
psiCpuSome% psiMemSome% psiMemFull% psiIoSome% psiIoFull%
```

`--view summary`, `--view io`, and other views reject `--pressure` with a clear
error. `--view summary` and `--view io` remain compact and are not expanded by
`--pressure`.

`--verbose` and `--pressure` can be used together on `--view system`. The table
keeps the verbose system columns in the `system` section and appends the PSI
columns in a separate `pressure` section:

```text
r/s w/s rkB/s wkB/s awaitS r_awaitS w_awaitS aqu-sz util% user_cpu% system_cpu% iowait% residentMB virtualMB ctxt/s swapIn/s swapOut/s steal% procsRun procsBlk memAvailMB cachedMB osDirtyMB heapMB allocMB dbFree% dbFreeGB | psiCpuSome% psiMemSome% psiMemFull% psiIoSome% psiIoFull%
```

### Reading `mongos` Disk Metrics

For `mongos`, disk columns in `--view system`, `--view summary`, and `--view io` come from
host-level FTDC `systemMetrics.disks.*` counters. They do not mean `mongos`
has a local storage engine or persists collection data like `mongod`.

Read them as machine-level IO pressure that may include:

- temporary spill activity from sorts or aggregation stages
- log writes
- swap activity
- IO from other colocated processes

That makes the disk metrics useful for `mongos`, but only as indirect signal.
High IO can point to spill-heavy router work or host contention; by itself it
does not prove that `mongos` is writing database files.

## Header

Before the metrics, the CLI prints the capture header as two-column tables.
Tables with nothing to show are omitted. `--json` carries the same facts in
`metadata` and `rsInfo` (see `--json`).

```text
Report                Path, View, Mode (with --stats/--findings), Interval or Average,
                      Samples, From, To, Process
Host                  Hostname, OS, Kernel, Architecture, CPU cores, Memory,
                      Max connections
Build                 Version, Git version, Modules, Storage engine, Allocator,
                      OpenSSL, Percona features
Replica Set           Set name, Member count, Node 1..N
Command Line Options  parsed startup options, one per row
Parameters            wtCache plus explicitly configured setParameter values
```

Example (Build and Command Line Options shortened):

```text
Report
+---------+--------------------------------+
| Field   | Value                          |
+---------+--------------------------------+
| Path    | testdata/diagnostic.data.27000 |
| View    | server                         |
| Average | 15m                            |
| Samples | 4                              |
| From    | 2026-06-04T23:00:01Z           |
| To      | 2026-06-04T23:59:59Z           |
| Process | mongod                         |
+---------+--------------------------------+

Host
+-----------------+-------------------+
| Field           | Value             |
+-----------------+-------------------+
| Hostname        | agusp:27000       |
| OS              | Ubuntu 24.04      |
| Kernel          | 6.8.0-110-generic |
| Architecture    | x86_64            |
| CPU cores       | 8                 |
| Memory          | 31 GiB            |
| Max connections | 409               |
+-----------------+-------------------+

Replica Set
+--------------+-----------------+
| Field        | Value           |
+--------------+-----------------+
| Set name     | rs0             |
| Member count | 3               |
| Node 1       | localhost:27000 |
| Node 2       | localhost:27001 |
| Node 3       | localhost:27002 |
+--------------+-----------------+

Parameters
+---------+-------+
| Field   | Value |
+---------+-------+
| wtCache | 0.5   |
+---------+-------+
```

### `Report`

`From` and `To` are the first and last rendered rows after `--from` / `--to`
filtering, not the requested bounds. `Samples` is the number of rendered rows
(buckets with `--avg`). `Process` comes from `serverStatus.process` (`mongod`
or `mongos`); when it is missing, output falls back to the mongod layout and
prints `mongod`.

### `Host`

`Max connections` is `connections.current + connections.available` from the
first usable `serverStatus` sample, derived during metadata reading. It prints
`-` when either field is missing in that sample.

### `Build`

`Percona features` is deduplicated, keeping first-seen order.

### `Replica Set`

Maps the generic `node1`, `node2`, ..., `nodeN` lag column labels (`Node 1`,
`Node 2`, ...) to the real member names. Member order prefers
`replSetGetConfig` order when available. Otherwise it uses first-seen order
from `replSetGetStatus.members` or from `serverStatus.repl.hosts`,
`serverStatus.repl.passives`, and `serverStatus.repl.arbiters`. The
`serverStatus.repl` fallback keeps shard data-node reports aligned with regular
replica-set node reports when FTDC does not include full replica-set config
metadata. If a new member appears later, it gets the next label, such as
`node4`; earlier rows show `-` for that member.

### `Command Line Options`

The parsed startup config from `getCmdLineOpts`, one dotted option per row.
It does not print raw `argv`, so config-file-driven launches do not show the
executable path or `-f /path/to/mongod.conf` wrapper arguments there.
`setParameter` values are listed under `Parameters` instead.

### `Parameters`

Always includes the configured WiredTiger cache size when available. Other
parameters are shown only when explicitly configured, under `setParameter`.

### Process markers

PID, uptime, and process start can change during a capture, so they are
printed as markers in the metrics table: before the first row, and wherever a
restart or a capture gap is detected. The process name follows the same
`serverStatus.process` detection:

```text
--- mongod process: pid=524587 start=2026-05-20T22:37:07Z ---
--- gap 1495s: rate baseline reset ---
--- mongod restart detected: pid=656311 start=2026-06-04T23:25:26Z ---
```

Rate baselines are reset after a restart or gap, so the first row after one
often shows `-` for rates. `--findings` lists the same events as a timeline.

Metadata changes across files are normal in rotated diagnostic directories and
are not printed as warnings.

## Column Reference

### Common

```text
datetime       sample time in UTC for FTDC sample rows; RFC3339 (`Z`)
rsState        current node replica-set state for that row, derived
```

`rsState` is selected from row-level metrics, not newest metadata. It uses
`serverStatus.repl.isWritablePrimary` / `ismaster`, `serverStatus.repl.secondary`,
`serverStatus.repl.myState`, or the current member in `replSetGetStatus.members`.
Allowed output values are:

```text
PRIMARY SECONDARY RECOVERING STARTUP2 ARBITER UNKNOWN
```

### `replication` Section

`replication` is shown before `server` in `--view summary`. The focused
replication view is `--view repl`.

```text
lagS     replication header label; the following node columns are lag values in seconds
node1    replication lag for Replica Set node1 in seconds, derived
node2    replication lag for Replica Set node2 in seconds, derived
nodeN    replication lag for Replica Set nodeN in seconds, derived
majLagS   majority commit lag in seconds, derived
rsState    current node replica-set state for that row, derived
hbMs      average member ping latency in milliseconds, verbose only
applyOps/s   replication apply throughput in ops/sec, verbose only
applyBufCnt replication apply buffer item count, verbose only
applyBufMB  replication apply buffer size in MiB, verbose only
replNetMB/s  oplog data fetched from the sync source, MiB/s, verbose only
oplogMB     oplog size in MiB, verbose only
oplogUsed%  oplog size as a percent of its configured maximum, verbose only
flowLagged  1 while flow control considers the majority commit point lagged, verbose only
flowRate    flow control target rate (lock acquisitions per second), only while flowLagged is 1, verbose only
flowWaitMS/s  time operations waited for flow control, ms per second, verbose only
```

Sources: `replSetGetStatus.members[].pingMs`,
`serverStatus.metrics.repl.apply.ops`,
`serverStatus.metrics.repl.buffer.apply.count`, and
`serverStatus.metrics.repl.buffer.apply.sizeBytes`.

```text
replNetMB/s   rate(serverStatus.metrics.repl.network.bytes) / 1024 / 1024
oplogMB       local.oplog.rs.stats.storageStats.size / 1024 / 1024
oplogUsed%    local.oplog.rs.stats.storageStats.size / local.oplog.rs.stats.storageStats.maxSize * 100
flowLagged    serverStatus.flowControl.isLagged
flowRate      serverStatus.flowControl.targetRateLimit, when isLagged is 1
flowWaitMS/s  rate(serverStatus.flowControl.timeAcquiringMicros) / 1000
```

FTDC does not record the first and last oplog entry times, so the oplog
window (hours of history) cannot be calculated from it. `oplogUsed%` below
100 means the oplog has not wrapped yet and still holds everything since it
was created; at about 100 it is full and the oldest entries are being
removed. Ask for `rs.printReplicationInfo()` output when the window matters.
`replNetMB/s` is only nonzero on members that fetch the oplog (secondaries).
`flowRate` is `-` while flow control is not throttling, because MongoDB keeps
a very large placeholder value there.

With `--verbose` on `--view repl`, the replication columns continue after
`majLagS` and `rsState` in the order shown above.

Table column names are always generic `node1..nodeN` labels, never replica-set
hostnames. The real member names are listed in the `Replica Set` header
table (`rsInfo` in JSON). The leading
`lagS` header marks the node columns as lag values in seconds and is not
repeated as a data literal on each row.

`majLagS` is the lag between `serverStatus.repl.lastWrite.lastWriteDate` and
`serverStatus.repl.lastWrite.majorityWriteDate`, in seconds. It appears after
the per-node lag columns in `--view summary` and `--view repl`.

Lag is calculated per row from that timestamp's `replSetGetStatus` data:

```text
memberLagS = primary.optimeDate - member.optimeDate
```

If `optimeDate` is unavailable, `optime.ts.t` is used as a fallback:

```text
memberLagS = primary.optime.ts.t - member.optime.ts.t
```

The PRIMARY member lag is `0.0`. Negative computed lag is clamped to `0.0`.
`-` means no PRIMARY was visible in that sample or the member had no usable
optime data.

### `server` View

```text
qTot     global lock queued operations total, raw
ins/s    inserts per second, rate
qry/s    queries per second, rate
upd/s    updates per second, rate
del/s    deletes per second, rate
getm/s   getMore operations per second, rate
cmd/s    commands per second, rate
rLatS    average read latency in seconds, derived
wLatS    average write latency in seconds, derived
cLatS    average command latency in seconds, derived
```

Verbose-only columns for `--view server --verbose`:

```text
rdQ          operations waiting for a read ticket, raw (7.0+)
wrQ          operations waiting for a write ticket, raw (7.0+)
rdQMS        average wait of operations that queued for a read ticket, milliseconds (7.0+)
wrQMS        average wait of operations that queued for a write ticket, milliseconds (7.0+)
wrConfl/s    write conflicts per second, rate
collScans/s  collection scans per second, rate
keys/ret     index keys examined per document returned, ratio
docs/ret     documents examined per document returned, ratio
txnActive    transactions currently running an operation, raw
txnCommit/s  transactions committed per second, rate
txnAbort/s   transactions aborted per second, rate
```

Server verbose sources:

```text
rdQ, wrQ        serverStatus.queues.execution.<read|write>.normalPriority.queueLength
rdQMS, wrQMS    delta(queues.execution.<read|write>.normalPriority.totalTimeQueuedMicros)
                / delta(queues.execution.<read|write>.normalPriority.addedToQueue) / 1000
wrConfl/s       rate(serverStatus.metrics.operation.writeConflicts)
collScans/s     rate(serverStatus.metrics.queryExecutor.collectionScans.total)
keys/ret        delta(serverStatus.metrics.queryExecutor.scanned)
                / delta(serverStatus.metrics.document.returned)
docs/ret        delta(serverStatus.metrics.queryExecutor.scannedObjects)
                / delta(serverStatus.metrics.document.returned)
txnActive       serverStatus.transactions.currentActive
txnCommit/s     rate(serverStatus.transactions.totalCommitted)
txnAbort/s      rate(serverStatus.transactions.totalAborted)
```

The admission-queue columns need MongoDB 7.0 or later (`queues.execution`);
older versions show `-`. `rdQMS` / `wrQMS` are `-` in rows where no operation
queued, and `keys/ret` / `docs/ret` are `-` in rows where no document was
returned. High `keys/ret` or `docs/ret` (hundreds or more) points to queries
that examine far more than they return, typically a missing or poor index; a
high `docs/ret` with `keys/ret` near 0 is a collection scan. On `mongos`,
only the transaction columns are reported; the others show `-`.

Latency formula:

```text
latency_s = delta(opLatencies.<type>.latency) / delta(opLatencies.<type>.ops) / 1000000
```

If the operation delta is zero, latency is undefined and prints `-`.

### `wt` View

Default columns:

```text
wtCache%    WiredTiger cache used percent, derived
dirty%      dirty bytes as percent of configured cache, derived
wtRdMB/s    bytes read into WT cache per second, rate
wtWrMB/s    bytes written from WT cache per second, rate
evict/s     pages evicted per second by all threads, rate
appEvict/s  pages evicted by application threads per second, rate
ckptMS      most recent WiredTiger checkpoint duration in milliseconds
rdTkt       WiredTiger read tickets available, raw
wrTkt       WiredTiger write tickets available, raw
```

Verbose-only columns for `--view wt --verbose`:

```text
cacheMB       current WT cache bytes used, converted to MB, derived raw gauge
dirtyMB       dirty bytes in WT cache, converted to MB, derived raw gauge
updatesMB     update bytes in WT cache, converted to MB, derived raw gauge
evictWalks/s  eviction walks per second, rate
evictBusy/s   pages skipped or blocked because busy, per second, rate
ckptPages/s   checkpoint pages written per second, rate
hsInsert/s    history store inserts per second, rate
hsRead/s      history store reads per second, rate
hsWriteMB/s   history store bytes written per second, converted to MB/s, rate
appEvictMS/s  time application threads spent evicting pages, ms per second
appReadMS/s   time application threads spent reading pages from disk into the cache, ms per second
logMB/s       journal (WiredTiger log) bytes written, MiB per second
logSyncMS     average journal sync duration, milliseconds
```

`appEvictMS/s` and `appReadMS/s` are time user operations spent on cache work
instead of their own: 1000 ms/s is one thread busy for the whole second.
Sustained values point to cache pressure (eviction) or a working set larger
than the cache (reads). `logSyncMS` rising with `w_awaitS` points to slow
journal storage.

`cacheMB`, `dirtyMB`, and `updatesMB` print as integer MB. `wtRdMB/s`,
`wtWrMB/s`, and `hsWriteMB/s` print with one decimal. WiredTiger verbose paths
vary across MongoDB and PSMDB versions; unavailable metrics render as `-`.

WiredTiger column sources:

```text
wtCache%    serverStatus.wiredTiger.cache.bytes currently in the cache
            / serverStatus.wiredTiger.cache.maximum bytes configured * 100
dirty%      serverStatus.wiredTiger.cache.tracked dirty bytes in the cache
            / serverStatus.wiredTiger.cache.maximum bytes configured * 100
cacheMB     serverStatus.wiredTiger.cache.bytes currently in the cache / 1024 / 1024
dirtyMB     serverStatus.wiredTiger.cache.tracked dirty bytes in the cache / 1024 / 1024
updatesMB   first available:
            serverStatus.wiredTiger.cache.bytes belonging to the updates in the cache
            serverStatus.wiredTiger.cache.tracked bytes belonging to the updates in the cache
            serverStatus.wiredTiger.cache.bytes allocated for updates
            then / 1024 / 1024
wtRdMB/s    rate(serverStatus.wiredTiger.cache.bytes read into cache) / 1024 / 1024
wtWrMB/s    rate(serverStatus.wiredTiger.cache.bytes written from cache) / 1024 / 1024
evict/s     rate sum, first available group:
            serverStatus.wiredTiger.cache.pages evicted by eviction server
            serverStatus.wiredTiger.cache.pages evicted by application threads
            fallback (newer WiredTiger):
            serverStatus.wiredTiger.cache.unmodified pages evicted
            serverStatus.wiredTiger.cache.modified pages evicted
appEvict/s  first available:
            rate(serverStatus.wiredTiger.cache.pages evicted by application threads)
            fallback (newer WiredTiger), clamped to zero:
            rate(serverStatus.wiredTiger.cache.page evict attempts by application threads)
            - rate(serverStatus.wiredTiger.cache.page evict failures by application threads)
evictWalks/s
            rate sum:
            serverStatus.wiredTiger.cache.eviction walks started from root of tree
            serverStatus.wiredTiger.cache.eviction walks started from saved location in tree
evictBusy/s rate sum:
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of active children on an internal page
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of failure in reconciliation
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of a cache overflow item
ckptMS      first available, already in milliseconds:
            serverStatus.wiredTiger.checkpoint.most recent time (msecs)
            serverStatus.wiredTiger.transaction.transaction checkpoint most recent time (msecs)
ckptPages/s first available rate:
            serverStatus.wiredTiger.transaction.transaction checkpoint pages written
            serverStatus.wiredTiger.checkpoint-cleanup.pages written
            serverStatus.wiredTiger.checkpoint.number of pages caused to be reconciled
rdTkt       first available:
            serverStatus.wiredTiger.concurrentTransactions.read.available
            serverStatus.queues.execution.read.available
wrTkt       first available:
            serverStatus.wiredTiger.concurrentTransactions.write.available
            serverStatus.queues.execution.write.available
hsInsert/s  first available rate:
            serverStatus.wiredTiger.cache.history store table insert calls
            serverStatus.wiredTiger.history store.history store table insert calls
hsRead/s    first available rate:
            serverStatus.wiredTiger.cache.history store table read calls
            serverStatus.wiredTiger.cache.history store table reads
            serverStatus.wiredTiger.history store.history store table read calls
hsWriteMB/s first available rate:
            serverStatus.wiredTiger.cache.bytes written from cache into history store
            serverStatus.wiredTiger.history store.history store table bytes written
            then / 1024 / 1024
appEvictMS/s
            rate(serverStatus.wiredTiger.cache.application thread time evicting (usecs)) / 1000
appReadMS/s rate(serverStatus.wiredTiger.cache.application threads page read from disk to cache time (usecs)) / 1000
logMB/s     rate(serverStatus.wiredTiger.log.log bytes written) / 1024 / 1024
logSyncMS   delta(serverStatus.wiredTiger.log.log sync time duration (usecs))
            / delta(serverStatus.wiredTiger.log.log sync operations) / 1000
```

### `io` View

`--view io` renders one top-level `io` section with one subgroup per device in
the terminal table. It uses the same compact disk metric set as the summary `io`
section and keeps devices in stable alphabetical order.

`--view io --json` emits nested per-device values at
`io.devices.<device>.<metric>`.

### `system` View

Default `--view system` columns:

```text
r/s         disk reads per second, rate
w/s         disk writes per second, rate
awaitS      average total disk wait in seconds, derived
r_awaitS    average read wait in seconds, derived
w_awaitS    average write wait in seconds, derived
aqu-sz      average queue size, derived
util%       disk utilization percent, derived
user_cpu%   MongoDB user CPU percent, derived
system_cpu% MongoDB system CPU percent, derived
iowait%     OS iowait CPU percent, derived
residentMB  MongoDB resident memory in MB, raw integer
virtualMB   MongoDB virtual memory in MB, raw integer
```

Verbose-only columns for `--view system --verbose`:

```text
rkB/s        disk read throughput in KiB/s, derived rate
wkB/s        disk write throughput in KiB/s, derived rate
ctxt/s       context switches per second, derived rate
swapIn/s     swap-ins per second, derived rate
swapOut/s    swap-outs per second, derived rate
steal%       CPU time stolen by the hypervisor, percent of total CPU time
procsRun     processes running or runnable, raw
procsBlk     processes blocked waiting for IO, raw
memAvailMB   host memory available for new allocations, MB
cachedMB     host page cache, MB
osDirtyMB    host page cache waiting to be written to disk, MB
heapMB       tcmalloc heap size of the process, MB
allocMB      bytes allocated by the process from the tcmalloc heap, MB
dbFree%      free space on the filesystem holding storage.dbPath, percent
dbFreeGB     free space on the filesystem holding storage.dbPath, GiB
```

Pressure-only columns for `--view system --pressure`:

```text
psiCpuSome%  CPU PSI pressure percent, derived
psiMemSome%  memory PSI pressure percent, derived
psiMemFull%  memory full PSI pressure percent, derived
psiIoSome%   IO PSI pressure percent, derived
psiIoFull%   IO full PSI pressure percent, derived
```

With both `--verbose` and `--pressure`, the verbose system columns stay in the
`system` section and PSI columns are appended in the separate `pressure`
section shown above.

Disk formulas:

```text
rkB/s     = delta(read_sectors)  * sector_size / 1024 / interval_seconds
wkB/s     = delta(write_sectors) * sector_size / 1024 / interval_seconds
util%     = delta(io_time_ms) / (interval_seconds * 1000) * 100
awaitS    = delta(io_queued_ms) / (delta(reads) + delta(writes)) / 1000
r_awaitS  = delta(read_time_ms) / delta(reads) / 1000
w_awaitS  = delta(write_time_ms) / delta(writes) / 1000
aqu-sz    = delta(io_queued_ms) / (interval_seconds * 1000)
```

`aqu-sz` is average queue size over the row interval, not a per-second rate.

When `serverStatus.extra_info.user_time_us` and
`serverStatus.extra_info.system_time_us` are available, `user_cpu%` and
`system_cpu%` are normalized by the available CPU count so the process stays
within total system capacity. If those counters are absent, `mongodb-ftdcstat` falls
back to the FTDC OS CPU split.

System verbose sources:

```text
ctxt/s       rate(systemMetrics.cpu.ctxt)
swapIn/s     rate(systemMetrics.vmstat.pswpin)
swapOut/s    rate(systemMetrics.vmstat.pswpout)
steal%       delta(systemMetrics.cpu.steal_ms) / delta(sum of all systemMetrics.cpu.*_ms) * 100
procsRun     systemMetrics.cpu.procs_running
procsBlk     systemMetrics.cpu.procs_blocked
memAvailMB   systemMetrics.memory.MemAvailable_kb / 1024
cachedMB     systemMetrics.memory.Cached_kb / 1024
osDirtyMB    systemMetrics.memory.Dirty_kb / 1024
heapMB       serverStatus.tcmalloc.generic.heap_size / 1024 / 1024
allocMB      serverStatus.tcmalloc.generic.current_allocated_bytes / 1024 / 1024
dbFree%      systemMetrics.mounts.<mount>.available / systemMetrics.mounts.<mount>.capacity * 100
dbFreeGB     systemMetrics.mounts.<mount>.available / 1024^3
```

`<mount>` is the longest mount point in the capture that contains
`storage.dbPath` from the startup options (`/data/db` when it is not set).
`available` is the space usable by non-root processes, as reported by the OS.
`dbFree%` and `dbFreeGB` are `-` on `mongos`, which has no data files, and when
no captured mount contains the dbPath. `heapMB - allocMB` is memory tcmalloc
holds but the process is not using; a large gap explains `residentMB` above the
WiredTiger cache size.

`ctxt/s` is context switches per second from the cumulative Linux context
switch counter (`/proc/stat` `ctxt` via FTDC `systemMetrics.cpu.ctxt`).

PSI sources:

```text
psi*%        prefer current avg10 from systemMetrics.pressure.<resource>.<scope>.avg10
             fallback to avg60 or avg300 if present
             otherwise derive interval percent from delta(totalMicros) / elapsedMicros * 100
```

PSI support is Linux-specific and depends on what FTDC captured. When `avg10`
is present it is used because it is the most useful short-window signal. Older
MongoDB/PSMDB builds or non-Linux captures may only have `totalMicros` or no
PSI metrics at all. Missing verbose or pressure metrics render as `-`.

### `network` View

Default `--view network` columns:

```text
activeConn      current active connections
idleConn        current - active, clamped to zero
totalCreated/s  new connections per second, derived rate
```

Verbose-only columns for `--view network --verbose`:

```text
queuedConn    current queued connections during establishment
rejConn/s     rejected connections per second, derived rate
dnsSlow/s     slow DNS operations per second, derived rate
tlsSlow/s     slow TLS/SSL operations per second, derived rate
netTimeout/s  network timeout events per second, derived rate
```

Network formulas and sources:

```text
maxConn         = firstSample.connections.current + firstSample.connections.available
activeConn      = serverStatus.connections.active
idleConn        = max(serverStatus.connections.current - serverStatus.connections.active, 0)
totalCreated/s  = delta(serverStatus.connections.totalCreated) / elapsed seconds
queuedConn      = serverStatus.connections.queuedForEstablishment
rejConn/s       = delta(serverStatus.connections.rejected) / elapsed seconds
dnsSlow/s       = delta(serverStatus.network.numSlowDNSOperations) / elapsed seconds
tlsSlow/s       = delta(serverStatus.network.numSlowSSLOperations) / elapsed seconds
netTimeout/s    = delta(serverStatus.metrics.operation.numConnectionNetworkTimeouts) / elapsed seconds
```

The network view intentionally excludes raw traffic volume, compression ratios,
request size averages, client disconnects, and ingress admission counters.
Those values are often hard to interpret without NIC, cloud, container, or OS
bandwidth context.

The `summary` view also includes the compact network section after
server metrics, using the same `activeConn`, `idleConn`, and `totalCreated/s`
columns. Compare `activeConn + idleConn` with `Max connections` in the `Host`
header table (`maxConn` above).

### `repl` View

`--view repl` renders only the `replication` section, including per-node lag
columns, `majLagS`, and `rsState`. It does not render `server`, `system`, or
`wiredTiger` columns. For `mongos`, it renders the `router` section instead.

## Numeric Formatting

```text
integer counters and raw integer values  no decimals
rates                                   one decimal
percentages                             one decimal
latencies and disk waits                three decimals
MiB values                              one decimal
boolean gauges                          0 or 1
```

Examples:

```text
conn=0       -> 0
ins/s=0      -> 0.0
user_cpu%=0  -> 0.0
awaitS=0     -> 0.000
wtCache%=0   -> 0.0
```

## Zero vs Missing

Display rules:

```text
0  means the value exists and is zero
-  means missing, unavailable, undefined, or cannot be computed
```

Examples:

```text
present counter with zero delta       -> 0
missing FTDC metric path              -> -
average with zero denominator         -> -
negative delta after counter reset    -> -
rate across process restart boundary  -> -
```

## Tests

```bash
go test ./...
```

Quick smoke test:

```bash
./mongodb-ftdcstat diagnostic.data --view summary --avg 15m
```
