# mongodb-ftdcstat metric reference

## Views and flags

`mongodb-ftdcstat` reads a MongoDB `diagnostic.data` directory, not a single FTDC file. It discovers `metrics.*`, `metrics.interim`, `interim*`, and exported JSON files, and treats them as one chronological capture.

Views:

- `summary`: one wide table in this order: `replication | server | network | system | wiredTiger`. Use for triage.
- `all`: compatibility alias for `summary`.
- `server`: serverStatus counters, latencies, and queues.
- `wt`: WiredTiger cache, eviction, checkpoint, and ticket metrics.
- `system`: CPU, memory, disk, swap, and optionally Linux PSI.
- `network`: connection activity and connection-establishment diagnostics.
- `repl`: replica-set lag and replication state.
- `disk`: compatibility alias for `system`.

Important flags:

- `--interval N`: display spacing in seconds. It does not aggregate metrics.
- `--avg DURATION`: average derived rows into fixed buckets. Valid bucket sizes are `1m` through `15m`; cannot be combined with explicit `--interval`.
- `--from ISO_TIME` / `--to ISO_TIME`: filter samples; `from` is inclusive and `to` is exclusive.
- `--device DEVICE`: filter disk-derived fields to one device.
- `--json`: output metadata, warnings, selected view, derived rows, and `rsInfo.members` mapping.
- `--web`: serve a local plotting UI and still print terminal output; cannot combine with `--json`.
- `--verbose`: expands only `repl`, `wt`, `system`, and `network`; not `summary`.
- `--pressure`: only supported for `--view system`; appends Linux PSI columns in a separate pressure section.

## Header fields

- `buildInfo`: version/build/storage/allocator/OpenSSL and Percona features when present.
- `rsInfo`: replica set name plus `node1..nodeN` to host:port mapping.
- `hostInfo`: hostname, OS, kernel, libc, CPU topology, memory, pages, THP, version string.
- `getCmdLineOpts`: parsed startup config flattened into `key=value` lines.
- `configured parameters`: configured values such as WiredTiger cache size.
- `metricsRange`: actual first and last rendered metric row timestamps after view selection and filtering.
- `network maxConn`: first usable sample's `connections.current + connections.available`, or `-` when unavailable.
- Process markers before rows show mongod PID/start and restarts. Rate baselines reset after detected restarts.

## Common display semantics

- `0`: present and zero.
- `-`: missing, unavailable, undefined, cannot be computed, zero denominator, negative delta after reset, or rate across restart boundary.
- JSON uses `null` for unavailable lag values and missing data.
- Formatting: integer counters have no decimals; rates and percentages usually one decimal; latencies/disk waits three decimals; MiB one decimal; booleans as `0` or `1`.

## Replication columns

Focused view: `--view repl`. In `summary`, replication appears first.

Columns:

- `lagS`: header label indicating following `node1..nodeN` columns are lag seconds.
- `node1..nodeN`: per-member replication lag in seconds using `rsInfo` labels.
- `majLagS`: majority commit lag in seconds.
- `rsState`: current node replica-set state for the row.
- Verbose only: `hbMs`, `applyOps/s`, `applyBufCnt`, `applyBufMB`.

Replica state values: `PRIMARY`, `SECONDARY`, `RECOVERING`, `STARTUP2`, `ARBITER`, `UNKNOWN`.

Per-member lag calculation:

```text
memberLagS = primary.optimeDate - member.optimeDate
```

Fallback:

```text
memberLagS = primary.optime.ts.t - member.optime.ts.t
```

PRIMARY lag is `0.0`. Negative lag is clamped to `0.0`. `-` means no PRIMARY was visible or the member lacked usable optime data.

`majLagS` is `serverStatus.repl.lastWrite.lastWriteDate - serverStatus.repl.lastWrite.majorityWriteDate` in seconds.

Verbose sources:

- `hbMs`: average `replSetGetStatus.members[].pingMs`.
- `applyOps/s`: rate of `serverStatus.metrics.repl.apply.ops`.
- `applyBufCnt`: `serverStatus.metrics.repl.buffer.apply.count`.
- `applyBufMB`: `serverStatus.metrics.repl.buffer.apply.sizeBytes / 1024 / 1024`.

Interpretation hints:

- Sustained nonzero `nodeN` lag indicates a lagging member; map `nodeN` through `rsInfo` before naming it.
- High `majLagS` points to majority-commit replication delay and can affect write concern majority.
- High `hbMs` supports network/host latency as a contributor, not proof by itself.
- Rising `applyBufCnt`/`applyBufMB` with nonzero lag suggests apply-side backlog.

## Server columns

Columns:

- `qTot`: global lock queued operations total, raw.
- `ins/s`, `qry/s`, `upd/s`, `del/s`, `getm/s`, `cmd/s`: operation rates.
- `rLatS`, `wLatS`, `cLatS`: average read/write/command latency in seconds.

Latency formula:

```text
latency_s = delta(opLatencies.<type>.latency) / delta(opLatencies.<type>.ops) / 1000000
```

If operation delta is zero, latency is undefined and prints `-`.

Interpretation hints:

- Rising latency with high throughput may indicate workload pressure; rising latency with low throughput often points elsewhere, such as queues, locks, disk, cache, or external dependencies.
- `qTot` is a queue snapshot; sustained elevation is more meaningful than a single spike.
- Command rate (`cmd/s`) can dominate in monitoring-heavy or metadata-heavy workloads.

## WiredTiger columns

Focused view: `--view wt`; use `--verbose` for deeper diagnostics.

Default columns:

- `wtCache%`: WT cache used percent = bytes currently in cache / maximum configured * 100.
- `dirty%`: dirty bytes / configured cache * 100.
- `wtRdMB/s`: bytes read into WT cache per second, MiB/s.
- `wtWrMB/s`: bytes written from WT cache per second, MiB/s.
- `evict/s`: pages evicted by eviction server/application/fallback groups per second.
- `appEvict/s`: application thread eviction/page-read count per second.
- `ckptMS`: most recent checkpoint duration in milliseconds.
- `rdTkt`, `wrTkt`: read/write tickets available.

Verbose columns:

- `cacheMB`: bytes currently in cache / MiB.
- `dirtyMB`: dirty bytes in cache / MiB.
- `updatesMB`: update bytes in cache / MiB, using the first available version-specific source.
- `evictWalks/s`: eviction walks per second.
- `evictBusy/s`: pages selected for eviction but unable to evict per second.
- `ckptPages/s`: checkpoint pages written per second.
- `hsInsert/s`, `hsRead/s`, `hsWriteMB/s`: history store insert/read/write rates.

Ticket fallbacks:

- `rdTkt`: first available of `serverStatus.wiredTiger.concurrentTransactions.read.available` or `serverStatus.queues.execution.read.available`.
- `wrTkt`: first available of `serverStatus.wiredTiger.concurrentTransactions.write.available` or `serverStatus.queues.execution.write.available`.

Interpretation hints:

- High `wtCache%` alone is not bad; WiredTiger cache is expected to be used.
- High `dirty%`, long `ckptMS`, high `wtWrMB/s`, and write latency together suggest checkpoint/write pressure.
- Low `rdTkt` or `wrTkt` sustained near zero can indicate ticket starvation or saturated concurrency.
- High `appEvict/s` means application threads are helping eviction, often a cache pressure symptom when sustained.
- High history store rates can indicate long-running transactions, pinned snapshots, or workload patterns that retain old versions.
- Missing WiredTiger verbose fields may be version-specific and should render as unknown, not healthy.

## System columns

Focused view: `--view system`; use `--verbose` and optionally `--pressure`.

Default columns:

- `r/s`, `w/s`: disk reads/writes per second.
- `awaitS`: average total disk wait seconds.
- `r_awaitS`, `w_awaitS`: average read/write wait seconds.
- `aqu-sz`: average queue size over the row interval.
- `util%`: disk utilization percent.
- `user_cpu%`, `system_cpu%`: MongoDB process CPU, normalized by CPU count when process CPU counters exist; otherwise OS CPU split fallback.
- `iowait%`: OS iowait CPU percent.
- `residentMB`, `virtualMB`: MongoDB memory gauges.

Verbose columns:

- `rkB/s`, `wkB/s`: disk read/write throughput in KiB/s.
- `ctxt/s`: Linux context switches per second from `systemMetrics.cpu.ctxt`.
- `swapIn/s`, `swapOut/s`: swap activity rates.

Disk formulas:

```text
rkB/s    = delta(read_sectors) * sector_size / 1024 / interval_seconds
wkB/s    = delta(write_sectors) * sector_size / 1024 / interval_seconds
util%    = delta(io_time_ms) / (interval_seconds * 1000) * 100
awaitS   = delta(io_queued_ms) / (delta(reads) + delta(writes)) / 1000
r_awaitS = delta(read_time_ms) / delta(reads) / 1000
w_awaitS = delta(write_time_ms) / delta(writes) / 1000
aqu-sz   = delta(io_queued_ms) / (interval_seconds * 1000)
```

Interpretation hints:

- High `util%` with high `awaitS` and queue depth suggests disk saturation.
- High `awaitS` without high `util%` can reflect burstiness, virtualization/storage latency, or mixed devices; inspect per-device with `--device`.
- High `iowait%` supports storage wait affecting CPU availability.
- High swap rates are usually important for MongoDB and should be called out.
- CPU percentages are process-normalized when possible; avoid comparing directly to top output unless CPU-count normalization is understood.

## Linux PSI pressure columns

Only with `--view system --pressure`.

- `psiCpuSome%`: CPU some pressure percent.
- `psiMemSome%`: memory some pressure percent.
- `psiMemFull%`: memory full pressure percent.
- `psiIoSome%`: IO some pressure percent.
- `psiIoFull%`: IO full pressure percent.

Source preference: `avg10`, fallback to `avg60`, fallback to `avg300`, otherwise derive interval percent from `delta(totalMicros) / elapsedMicros * 100`.

Interpretation hints:

- PSI is Linux-specific and depends on what FTDC captured.
- Sustained nonzero memory full or IO full pressure is usually stronger evidence than CPU some pressure alone.
- Use PSI alongside latency, queue, and throughput metrics; do not diagnose solely from PSI.

## Network columns

Focused view: `--view network`; use `--verbose` for connection-establishment symptoms.

Default columns:

- `activeConn`: `serverStatus.connections.active`.
- `idleConn`: `connections.current - connections.active`, clamped to zero.
- `totalCreated/s`: new connections per second from `delta(connections.totalCreated) / elapsed seconds`.

Verbose columns:

- `queuedConn`: `serverStatus.connections.queuedForEstablishment`.
- `rejConn/s`: rejected connections per second.
- `dnsSlow/s`: slow DNS operations per second.
- `tlsSlow/s`: slow TLS/SSL operations per second.
- `netTimeout/s`: connection network timeout events per second.

Network formulas:

```text
maxConn        = firstSample.connections.current + firstSample.connections.available
activeConn     = serverStatus.connections.active
idleConn       = max(serverStatus.connections.current - serverStatus.connections.active, 0)
totalCreated/s = delta(serverStatus.connections.totalCreated) / elapsed seconds
queuedConn     = serverStatus.connections.queuedForEstablishment
rejConn/s      = delta(serverStatus.connections.rejected) / elapsed seconds
dnsSlow/s      = delta(serverStatus.network.numSlowDNSOperations) / elapsed seconds
tlsSlow/s      = delta(serverStatus.network.numSlowSSLOperations) / elapsed seconds
netTimeout/s   = delta(serverStatus.metrics.operation.numConnectionNetworkTimeouts) / elapsed seconds
```

Interpretation hints:

- High `totalCreated/s` with many idle connections may indicate connection churn or missing pooling.
- Nonzero `queuedConn`, `rejConn/s`, `dnsSlow/s`, `tlsSlow/s`, or `netTimeout/s` points toward connection-establishment pressure.
- Raw traffic volume, compression ratios, client disconnects, and ingress admission counters are intentionally excluded from this view.

## Quick diagnostic patterns

### Replication bottleneck

Look for sustained nonzero `nodeN` lag, `majLagS`, state changes, high `hbMs`, rising apply buffer, and related system pressure.

Next command:

```bash
mongodb-ftdcstat diagnostic.data --view repl --verbose --avg 1m
```

### Disk bottleneck

Look for high `util%`, high `awaitS`/`r_awaitS`/`w_awaitS`, rising `aqu-sz`, high `iowait%`, and corresponding server or WT latency.

Next commands:

```bash
mongodb-ftdcstat diagnostic.data --view system --verbose --avg 1m
mongodb-ftdcstat diagnostic.data --view system --verbose --device <device> --avg 1m
```

### WiredTiger cache pressure

Look for high dirty/cache pressure, high `appEvict/s`, low tickets, high checkpoint duration, high history store activity, and latency increases.

Next command:

```bash
mongodb-ftdcstat diagnostic.data --view wt --verbose --avg 1m
```

### Connection churn or establishment pressure

Look for high `totalCreated/s`, nonzero `queuedConn`, `rejConn/s`, DNS/TLS slow rates, and network timeouts.

Next command:

```bash
mongodb-ftdcstat diagnostic.data --view network --verbose --avg 1m
```

### Need visual exploration

Use web mode for local charts, with aggregation or time filters for large captures:

```bash
mongodb-ftdcstat diagnostic.data --web --view summary --avg 5m
```
