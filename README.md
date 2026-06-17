# ftdcstat

`ftdcstat` reads a MongoDB `diagnostic.data` directory and prints FTDC metrics
as terminal-friendly tables.

## Build

```bash
go build -o ftdcstat ./cmd/ftdcstat
```

## Usage

```bash
ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--verbose] [--pressure]
```

The input is a directory, not a single FTDC file. The tool discovers
`metrics.*`, `metrics.interim`, `interim*`, and exported JSON files, then treats
them as one chronological capture.

## Options

### `<path-to-diagnostic-data-directory>`

Required. Path to a MongoDB FTDC diagnostic data directory.

### `--view server|wt|system|network|repl|summary|all`

Default: `summary`.

Views:

```text
server   Replica-set status plus MongoDB serverStatus counters, latency, queues, connections
wt       WiredTiger cache, eviction, checkpoint, and ticket metrics
system   CPU, memory, and disk metrics
network  Connection activity and network-establishment diagnostics
repl     Replica-set lag and replication state
summary  One wide table containing replication, server, network, system, and WiredTiger columns
all      Compatibility alias for summary
```

`disk` is accepted as a compatibility alias for `system`.

`--view summary` is intended for horizontal scrolling:

```bash
ftdcstat diagnostic.data --view summary | less -S
```

It prints one wide table, one row per display interval, and repeats the compact
section-label row plus the column header every 50 data rows. The wide table uses
`|` separators after `datetime` and between logical `replication`, `server`,
`network`, `system`, and `wiredTiger` groups. It avoids the old full-width dashed banner
lines. The `--view summary` section order is:

```text
replication | server | network | system | wiredTiger
```

### `--interval N`

Default: `60`.

`--interval` controls display spacing in seconds. It does not aggregate metrics
into buckets. FTDC samples are reconstructed in chronological order, rates are
calculated from adjacent raw samples, and rows are printed no more often than
the selected interval.

With the default MongoDB `diagnosticDataCollectionPeriodMillis=1000`,
`--interval 60` usually means about one line per minute.

### `--device DEVICE`

Filters disk-derived fields to a single device.

```bash
ftdcstat diagnostic.data --view system --device sda
```

Without `--device`, disk rates are totals across discovered disks and `util%` is
the maximum utilization among disks.

### `--from ISO_TIME` and `--to ISO_TIME`

Filters samples to a time range.

```bash
ftdcstat diagnostic.data --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"
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

### `--verbose`

`--verbose` expands columns for focused views only. It applies to `--view repl`,
`--view wt`, `--view system`, and `--view network`. It does not apply to `--view summary` or
`--view all`, which always print the compact rollup across replication, server,
network, system, and WiredTiger.

When used with `--view repl`, `--verbose` adds replication apply/buffer metrics
after `majLagS`:

```text
lagS node1 node2 ... nodeN majLagS hbMs applyOps/s applyBufCnt applyBufMB
```

When used with `--view wt`, `--verbose` expands the WiredTiger columns with
cache, eviction, checkpoint, ticket, and history store diagnostics:

```text
wtCache% dirty% cacheMB dirtyMB updatesMB wtRdMB/s wtWrMB/s evict/s appEvict/s evictWalks/s evictBusy/s ckptMS ckptPages/s rdTkt wrTkt hsInsert/s hsRead/s hsWriteMB/s
```

When used with `--view system`, `--verbose` adds disk throughput, context switch
rate, and swap activity after the default system columns:

```text
r/s w/s rkB/s wkB/s awaitS r_awaitS w_awaitS aqu-sz util% user_cpu% system_cpu% iowait% residentMB virtualMB ctxt/s swapIn/s swapOut/s
```

When used with `--view network`, `--verbose` appends queued and connection-establishment
symptom counters after the default network columns:

```text
activeConn idleConn totalCreated/s queuedConn rejConn/s dnsSlow/s tlsSlow/s netTimeout/s
```

Non-verbose output for all views is unchanged.

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

FTDC path selection adds only the explicit verbose replication paths when
`--verbose` is enabled for `--view repl`.

For `--view wt --verbose`, FTDC path selection adds only the explicit verbose
WiredTiger paths needed for the extra columns.

For `--view system --verbose`, FTDC path selection adds only the explicit
verbose system paths needed for `ctxt/s`, `swapIn/s`, and `swapOut/s`.

For `--view network --verbose`, FTDC path selection adds only the explicit
network-establishment paths needed for `queuedConn`, `rejConn/s`, `dnsSlow/s`,
`tlsSlow/s`, and `netTimeout/s`.

### `--pressure`

`--pressure` is only supported for `--view system`. It appends Linux PSI columns
in a separate `pressure` section after the system columns:

```text
psiCpuSome% psiMemSome% psiMemFull% psiIoSome% psiIoFull%
```

`--view summary`, `--view all`, and other views reject `--pressure` with a clear
error. `--view summary` and `--view all` remain compact and are not expanded by
`--pressure`.

For `--view system --pressure`, FTDC path selection adds only the explicit PSI
paths needed for the pressure columns.

`--verbose` and `--pressure` can be used together on `--view system`. The table
keeps the verbose system columns in the `system` section and appends the PSI
columns in a separate `pressure` section:

```text
r/s w/s rkB/s wkB/s awaitS r_awaitS w_awaitS aqu-sz util% user_cpu% system_cpu% iowait% residentMB virtualMB ctxt/s swapIn/s swapOut/s | psiCpuSome% psiMemSome% psiMemFull% psiIoSome% psiIoFull%
```

## Header

The static header prints stable environment data:

```text
buildInfo version/build/storage/allocator/OpenSSL, plus perconaFeatures on a separate line when present
rsInfo replica set name and node label to host:port mapping
hostInfo hostname/OS/kernel/libc/CPU topology/memory/pages/THP/versionString
getCmdLineOpts argv
configured parameters
network maxConn metadata for `--view network`
```

`buildInfo.perconaFeatures` is deduplicated while preserving first-seen order
and is printed on its own line when present.

`rsInfo` maps generic `node1`, `node2`, ..., `nodeN` labels to the real
replica-set member names. It prints one mapping line per member:

```text
rsInfo
  set=rs0 members:
    node1=localhost:27000
    node2=localhost:27001
    node3=localhost:27002
```

Member order prefers `replSetGetConfig` order when available. Otherwise it uses
first-seen order from `replSetGetStatus.members`. If a new member appears later,
it gets the next label, such as `node4`; earlier rows show `-` for that member.

`getCmdLineOpts` prints `argv` with the executable on the first line and each
option on its own indented line. Options with a following value are kept
together on the same line.

Process information is not printed in the static header because `pid`, uptime,
and process start can change over a capture. Instead, process markers are
printed immediately before the first metric row and after restarts:

```text
--- mongod process: pid=40955 start=2026-06-07T00:40:13-03:00 ---
--- mongod restart detected: pid=41234 start=2026-06-07T04:12:09-03:00 ---
```

Rate baselines are reset after a detected restart.

The `Parameters` section always includes configured WiredTiger cache size when
available. Other parameters are shown only when explicitly configured in startup
options or config-derived `getCmdLineOpts`, primarily under `setParameter`.

Metadata changes across files are normal in rotated diagnostic directories and
are not printed as warnings by default.

For `--view network`, the header also includes:

```text
network
  maxConn: connections.current + connections.available from the first usable serverStatus sample
```

If either connection field is unavailable in that first sample, `maxConn` is
printed as `-`.

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

`replication` is shown before `server` in `--view summary`. It is also included with
`--view server`, because replication lag is server/replica-set status data.

```text
lagS     replication header label; the following node columns are lag values in seconds
node1    replication lag for rsInfo node1 in seconds, derived
node2    replication lag for rsInfo node2 in seconds, derived
nodeN    replication lag for rsInfo nodeN in seconds, derived
majLagS   majority commit lag in seconds, derived
hbMs      average member ping latency in milliseconds, verbose only
applyOps/s   replication apply throughput in ops/sec, verbose only
applyBufCnt replication apply buffer item count, verbose only
applyBufMB  replication apply buffer size in MiB, verbose only
```

Sources: `replSetGetStatus.members[].pingMs`,
`serverStatus.metrics.repl.apply.ops`,
`serverStatus.metrics.repl.buffer.apply.count`, and
`serverStatus.metrics.repl.buffer.apply.sizeBytes`.

With `--verbose` on `--view repl`, the replication columns continue after
`majLagS` in the order shown above.

Table column names are always generic `node1..nodeN` labels, never replica-set
hostnames. The real member names are listed in the `rsInfo` header. The leading
`lagS` header marks the node columns as lag values in seconds and is not
repeated as a data literal on each row.

`majLagS` is the lag between `serverStatus.repl.lastWrite.lastWriteDate` and
`serverStatus.repl.lastWrite.majorityWriteDate`, in seconds. It appears after
the per-node lag columns in `--view summary`, `--view server`, and `--view repl`.

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
rsState  current node replica-set state for that row, derived
conn     current connections, raw
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

The old scalar `server.lagS` column was removed to avoid duplicating the
per-member `replication` section. `rsState` is the first `server` column.

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
evict/s     pages evicted by application threads per second, rate
appEvict/s  application thread eviction/page-read count per second, rate
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
```

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
            fallback:
            serverStatus.wiredTiger.cache.pages evicted because they exceeded the in-memory maximum
            serverStatus.wiredTiger.cache.unmodified pages evicted
            serverStatus.wiredTiger.cache.modified pages evicted
            fallback:
            serverStatus.wiredTiger.cache.eviction server candidate queue empty when topping up
            serverStatus.wiredTiger.cache.eviction server candidate queue not empty when topping up
appEvict/s  rate(serverStatus.wiredTiger.cache.application threads page read from disk to cache count)
evictWalks/s
            rate sum:
            serverStatus.wiredTiger.cache.eviction walks started from root of tree
            serverStatus.wiredTiger.cache.eviction walks started from saved location in tree
evictBusy/s rate sum:
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of active children on an internal page
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of failure in reconciliation
            serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of a cache overflow item
ckptMS      first available:
            serverStatus.wiredTiger.transaction.transaction checkpoint most recent duration for gathering all handles (usecs)
            serverStatus.wiredTiger.checkpoint.most recent duration for gathering all handles (usecs)
            then / 1000
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
```

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
within total system capacity. If those counters are absent, `ftdcstat` falls
back to the FTDC OS CPU split.

System verbose sources:

```text
ctxt/s       rate(systemMetrics.cpu.ctxt)
swapIn/s     rate(systemMetrics.vmstat.pswpin)
swapOut/s    rate(systemMetrics.vmstat.pswpout)
```

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

The `summary` and `all` views also include the compact network section after
server metrics, using the same `activeConn`, `idleConn`, and `totalCreated/s`
columns. The header always includes the `network` section and `maxConn`
metadata when the first usable `serverStatus` sample has both connection counts.
MongoDB/PSMDB builds or non-Linux captures may only have `totalMicros` or no
PSI metrics at all. Missing verbose or pressure metrics render as `-`.

Missing fields render as `-` in terminal output and `null` in JSON.

### `repl` View

`--view repl` is a compatibility alias that renders only the `replication`
section, including per-node lag columns and `majLagS`. It does not render
`server`, `system`, or `wiredTiger` columns. `rsState` remains in the `server`
section when using `--view server` or `--view summary`.

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
./ftdcstat diagnostic.data --view summary --interval 43200
```
