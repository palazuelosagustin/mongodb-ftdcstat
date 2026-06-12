# ftdcstat

`ftdcstat` reads a MongoDB `diagnostic.data` directory and prints FTDC metrics
as terminal-friendly tables.

## Build

```bash
go build -o ftdcstat ./cmd/ftdcstat
```

## Usage

```bash
ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|repl|all] [--interval N] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json]
```

The input is a directory, not a single FTDC file. The tool discovers
`metrics.*`, `metrics.interim`, `interim*`, and exported JSON files, then treats
them as one chronological capture.

## Options

### `<path-to-diagnostic-data-directory>`

Required. Path to a MongoDB FTDC diagnostic data directory.

### `--view server|wt|system|repl|all`

Default: `all`.

Views:

```text
server  Replica-set status plus MongoDB serverStatus counters, latency, queues, connections
wt      WiredTiger cache, eviction, checkpoint, and ticket metrics
system  CPU, memory, and disk metrics
repl    Replica-set lag and replication state
all     One wide table containing replication, server, system, and WiredTiger columns
```

`summary` is not a supported view. `disk` is accepted as a compatibility alias
for `system`.

`--view all` is intended for horizontal scrolling:

```bash
ftdcstat diagnostic.data --view all | less -S
```

It prints one wide table, one row per display interval, and repeats the compact
section-label row plus the column header every 50 data rows. The wide table uses
`|` separators after `datetime` and between logical `replication`, `server`,
`system`, and `wiredTiger` groups. It avoids the old full-width dashed banner
lines. The `--view all` section order is:

```text
replication | server | system | wiredTiger
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

## Header

The static header prints stable environment data:

```text
buildInfo version/build/storage/allocator/OpenSSL, plus perconaFeatures on a separate line when present
rsInfo replica set name and node label to host:port mapping
hostInfo hostname/OS/kernel/libc/CPU topology/memory/pages/THP/versionString
getCmdLineOpts argv
configured parameters
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

`replication` is shown before `server` in `--view all`. It is also included with
`--view server`, because replication lag is server/replica-set status data.

```text
lagS     replication header label; the following node columns are lag values in seconds
node1    replication lag for rsInfo node1 in seconds, derived
node2    replication lag for rsInfo node2 in seconds, derived
nodeN    replication lag for rsInfo nodeN in seconds, derived
majLagS   majority commit lag in seconds, derived
```

Table column names are always generic `node1..nodeN` labels, never replica-set
hostnames. The real member names are listed in the `rsInfo` header. The leading
`lagS` header marks the node columns as lag values in seconds and is not
repeated as a data literal on each row.

`majLagS` is the lag between `serverStatus.repl.lastWrite.lastWriteDate` and
`serverStatus.repl.lastWrite.majorityWriteDate`, in seconds. It appears after
the per-node lag columns in `--view all`, `--view server`, and `--view repl`.

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

### `system` View

Disk:

```text
r/s         disk reads per second, rate
w/s         disk writes per second, rate
rkB/s       disk read throughput in KiB/s, derived
wkB/s       disk write throughput in KiB/s, derived
r_awaitS    average read wait in seconds, derived
w_awaitS    average write wait in seconds, derived
awaitS      average total disk wait in seconds, derived
aqu-sz      average queue size, derived
util%       disk utilization percent, derived
user_cpu%   MongoDB user CPU percent, derived
system_cpu% MongoDB system CPU percent, derived
iowait%     OS iowait CPU percent, derived
residentMB  MongoDB resident memory in MB, raw
virtualMB   MongoDB virtual memory in MB, raw
```

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

When `serverStatus.extra_info.user_time_us` and
`serverStatus.extra_info.system_time_us` are available, `user_cpu%` and
`system_cpu%` are normalized by the available CPU count so the process stays
within total system capacity. If those counters are absent, `ftdcstat` falls
back to the FTDC OS CPU split.

Missing fields render as `-` in terminal output and `null` in JSON.

### `repl` View

`--view repl` is a compatibility alias that renders only the `replication`
section, including per-node lag columns and `majLagS`. It does not render
`server`, `system`, or `wiredTiger` columns. `rsState` remains in the `server`
section when using `--view server` or `--view all`.

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
./ftdcstat diagnostic.data --view all --interval 43200
```
