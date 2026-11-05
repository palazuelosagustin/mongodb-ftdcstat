package render

// columnDoc documents one column for --list-columns. Every registry column
// must have an entry (enforced by TestEveryColumnIsDocumented).
type columnDoc struct {
	Unit    string
	Meaning string
	Source  string
}

// nodeLagDoc documents the per-member lag columns node1..nodeN.
var nodeLagDoc = columnDoc{
	Unit:    "s",
	Meaning: "replication lag of replica-set member N (see the Replica Set header table)",
	Source:  "primary.optimeDate - member.optimeDate from replSetGetStatus.members (optime.ts.t fallback); PRIMARY is 0, negatives clamped to 0",
}

var columnDocs = map[string]columnDoc{
	// server
	"qTot":        {"count", "operations queued for the global lock", "serverStatus.globalLock.currentQueue.total"},
	"ins/s":       {"/s", "inserts per second", "rate(serverStatus.opcounters.insert)"},
	"qry/s":       {"/s", "queries per second", "rate(serverStatus.opcounters.query)"},
	"upd/s":       {"/s", "updates per second", "rate(serverStatus.opcounters.update)"},
	"del/s":       {"/s", "deletes per second", "rate(serverStatus.opcounters.delete)"},
	"getm/s":      {"/s", "getMore operations per second", "rate(serverStatus.opcounters.getmore)"},
	"cmd/s":       {"/s", "commands per second", "rate(serverStatus.opcounters.command)"},
	"rLatS":       {"s", "average read latency", "delta(opLatencies.reads.latency) / delta(opLatencies.reads.ops) / 1000000"},
	"wLatS":       {"s", "average write latency", "delta(opLatencies.writes.latency) / delta(opLatencies.writes.ops) / 1000000"},
	"cLatS":       {"s", "average command latency", "delta(opLatencies.commands.latency) / delta(opLatencies.commands.ops) / 1000000"},
	"rdQ":         {"count", "operations waiting for a read ticket (7.0+)", "queues.execution.read.normalPriority.queueLength (8.0) or wiredTiger.concurrentTransactions.read.queueLength (7.0)"},
	"wrQ":         {"count", "operations waiting for a write ticket (7.0+)", "queues.execution.write.normalPriority.queueLength (8.0) or wiredTiger.concurrentTransactions.write.queueLength (7.0)"},
	"rdQMS":       {"ms", "average wait of operations that queued for a read ticket (7.0+)", "delta(totalTimeQueuedMicros) / delta(addedToQueue) / 1000 of the read queue above"},
	"wrQMS":       {"ms", "average wait of operations that queued for a write ticket (7.0+)", "delta(totalTimeQueuedMicros) / delta(addedToQueue) / 1000 of the write queue above"},
	"wrConfl/s":   {"/s", "write conflicts per second", "rate(serverStatus.metrics.operation.writeConflicts)"},
	"collScans/s": {"/s", "collection scans per second", "rate(serverStatus.metrics.queryExecutor.collectionScans.total)"},
	"keys/ret":    {"ratio", "index keys examined per document returned; high means a poor or missing index", "delta(metrics.queryExecutor.scanned) / delta(metrics.document.returned)"},
	"docs/ret":    {"ratio", "documents examined per document returned; high with keys/ret near 0 is a collection scan", "delta(metrics.queryExecutor.scannedObjects) / delta(metrics.document.returned)"},
	"txnActive":   {"count", "transactions currently running an operation", "serverStatus.transactions.currentActive"},
	"txnCommit/s": {"/s", "transactions committed per second", "rate(serverStatus.transactions.totalCommitted)"},
	"txnAbort/s":  {"/s", "transactions aborted per second", "rate(serverStatus.transactions.totalAborted)"},

	// replication
	"rsState":      {"state", "this node's replica-set state (PRIMARY, SECONDARY, RECOVERING, STARTUP2, ARBITER, UNKNOWN)", "serverStatus.repl.isWritablePrimary/ismaster, .secondary, .myState, or the self member of replSetGetStatus"},
	"majLagS":      {"s", "majority commit lag: how far the majority commit point is behind the last write", "(serverStatus.repl.lastWrite.lastWriteDate - majorityWriteDate) / 1000"},
	"hbMs":         {"ms", "average heartbeat ping to the other members", "average of replSetGetStatus.members[].pingMs"},
	"applyOps/s":   {"/s", "oplog entries applied per second (secondaries)", "rate(serverStatus.metrics.repl.apply.ops)"},
	"applyBufCnt":  {"count", "oplog entries fetched and waiting to be applied", "metrics.repl.buffer.apply.count (8.0) or metrics.repl.buffer.count"},
	"applyBufMB":   {"MiB", "size of the fetched-but-not-applied oplog buffer", "metrics.repl.buffer.apply.sizeBytes (8.0) or metrics.repl.buffer.sizeBytes, / 1024 / 1024"},
	"replNetMB/s":  {"MiB/s", "oplog data fetched from the sync source (secondaries)", "rate(serverStatus.metrics.repl.network.bytes) / 1024 / 1024"},
	"fetchBatchKB": {"KiB", "average oplog batch fetched from the sync source; 16384 = full batches, the member is behind", "delta(metrics.repl.network.bytes) / delta(metrics.repl.network.getmores.num) / 1024"},
	"fetchMS":      {"ms", "average time per oplog batch fetched from the sync source", "delta(metrics.repl.network.getmores.totalMillis) / delta(metrics.repl.network.getmores.num)"},
	"fetchEmpty/s": {"/s", "empty oplog batches per second: the member is caught up and waiting", "rate(metrics.repl.network.getmores.numEmptyBatches)"},
	"oplogServe/s": {"/s", "oplog batches this member served to members syncing from it, per second", "rate(metrics.repl.network.oplogGetMoresProcessed.num)"},
	"oplogServeMS": {"ms", "average time this member took to serve one oplog batch, all fetching members together", "delta(oplogGetMoresProcessed.totalMillis) / delta(oplogGetMoresProcessed.num)"},
	"oplogMB":      {"MiB", "oplog size", "local.oplog.rs.stats.storageStats.size / 1024 / 1024"},
	"oplogUsed%":   {"%", "oplog size as a percent of its maximum; below 100 it has not wrapped (FTDC has no oplog window)", "storageStats.size / storageStats.maxSize * 100 of local.oplog.rs.stats"},
	"flowLagged":   {"0/1", "1 while flow control considers the majority commit point lagged", "serverStatus.flowControl.isLagged"},
	"flowRate":     {"/s", "flow control target rate (lock acquisitions per second), only while flowLagged is 1", "serverStatus.flowControl.targetRateLimit"},
	"flowWaitMS/s": {"ms/s", "time operations waited for flow control", "rate(serverStatus.flowControl.timeAcquiringMicros) / 1000"},

	// router (mongos)
	"nodes":      {"count", "hosts in the router's replica-set ping table", "count of router.connPoolStats.replicaSetPingTimesMillis.*"},
	"pingMS":     {"ms", "average ping to hosts with a real ping time; unpinged hosts are skipped", "average of router.connPoolStats.replicaSetPingTimesMillis.* below 1e12"},
	"helloOps/s": {"/s", "replica-set monitor hello calls per second", "rate(router.connPoolStats.replicaSetMonitor.hello.totalCalls)"},
	"helloMS":    {"ms", "average replica-set monitor hello latency; '-' when long-lived monitoring calls closed", "delta(hello.totalLatencyMicros) / delta(hello.totalCalls) / 1000"},
	"ghaOps/s":   {"/s", "getHostAndRefresh calls per second (requests waiting for a suitable host)", "rate(router.connPoolStats.replicaSetMonitor.getHostAndRefresh.totalCalls)"},
	"ghaMS":      {"ms", "average getHostAndRefresh latency: how long callers waited for a host", "delta(getHostAndRefresh.totalLatencyMicros) / delta(getHostAndRefresh.totalCalls) / 1000"},

	// system
	"user_cpu%":   {"%", "mongod/mongos user CPU, normalized by CPU count", "rate(serverStatus.extra_info.user_time_us) / CPU count; OS user+nice share as fallback"},
	"system_cpu%": {"%", "mongod/mongos system CPU, normalized by CPU count", "rate(serverStatus.extra_info.system_time_us) / CPU count; OS system+irq+softirq share as fallback"},
	"iowait%":     {"%", "host CPU time waiting for IO", "delta(systemMetrics.cpu.iowait_ms) / delta(all systemMetrics.cpu.*_ms) * 100"},
	"residentMB":  {"MB", "process resident memory", "serverStatus.mem.resident"},
	"virtualMB":   {"MB", "process virtual memory", "serverStatus.mem.virtual"},
	"ctxt/s":      {"/s", "host context switches per second", "rate(systemMetrics.cpu.ctxt)"},
	"swapIn/s":    {"/s", "pages swapped in per second", "rate(systemMetrics.vmstat.pswpin)"},
	"swapOut/s":   {"/s", "pages swapped out per second", "rate(systemMetrics.vmstat.pswpout)"},
	"steal%":      {"%", "CPU time stolen by the hypervisor", "delta(systemMetrics.cpu.steal_ms) / delta(all systemMetrics.cpu.*_ms) * 100"},
	"procsRun":    {"count", "processes running or runnable", "systemMetrics.cpu.procs_running"},
	"procsBlk":    {"count", "processes blocked waiting for IO", "systemMetrics.cpu.procs_blocked"},
	"memAvailMB":  {"MB", "host memory available for new allocations", "systemMetrics.memory.MemAvailable_kb / 1024"},
	"cachedMB":    {"MB", "host page cache", "systemMetrics.memory.Cached_kb / 1024"},
	"osDirtyMB":   {"MB", "host page cache waiting to be written to disk", "systemMetrics.memory.Dirty_kb / 1024"},
	"heapMB":      {"MB", "tcmalloc heap size of the process", "serverStatus.tcmalloc.generic.heap_size / 1024 / 1024"},
	"allocMB":     {"MB", "memory allocated by the process from the tcmalloc heap; heapMB - allocMB is held but unused", "serverStatus.tcmalloc.generic.current_allocated_bytes / 1024 / 1024"},
	"dbFree%":     {"%", "free space on the filesystem holding storage.dbPath", "systemMetrics.mounts.<mount>.available / .capacity * 100, <mount> = longest mount containing dbPath"},
	"dbFreeGB":    {"GiB", "free space on the filesystem holding storage.dbPath", "systemMetrics.mounts.<mount>.available / 1024^3"},
	"psiCpuSome%": {"%", "Linux PSI: share of time some tasks waited for CPU", "systemMetrics.pressure.cpu.some avg10 (avg60/avg300, or totalMicros rate, as fallback)"},
	"psiMemSome%": {"%", "Linux PSI: share of time some tasks waited for memory", "systemMetrics.pressure.memory.some avg10 (fallbacks as psiCpuSome%)"},
	"psiMemFull%": {"%", "Linux PSI: share of time all tasks waited for memory", "systemMetrics.pressure.memory.full avg10 (fallbacks as psiCpuSome%)"},
	"psiIoSome%":  {"%", "Linux PSI: share of time some tasks waited for IO", "systemMetrics.pressure.io.some avg10 (fallbacks as psiCpuSome%)"},
	"psiIoFull%":  {"%", "Linux PSI: share of time all tasks waited for IO", "systemMetrics.pressure.io.full avg10 (fallbacks as psiCpuSome%)"},

	// io
	"r/s":      {"/s", "disk reads per second", "rate(systemMetrics.disks.<dev>.reads)"},
	"w/s":      {"/s", "disk writes per second", "rate(systemMetrics.disks.<dev>.writes)"},
	"rkB/s":    {"KiB/s", "disk read throughput", "delta(read_sectors) * sector size / 1024 / seconds"},
	"wkB/s":    {"KiB/s", "disk write throughput", "delta(write_sectors) * sector size / 1024 / seconds"},
	"awaitS":   {"s", "average time per disk IO, queueing included", "delta(io_queued_ms) / (delta(reads) + delta(writes)) / 1000"},
	"r_awaitS": {"s", "average time per disk read", "delta(read_time_ms) / delta(reads) / 1000"},
	"w_awaitS": {"s", "average time per disk write", "delta(write_time_ms) / delta(writes) / 1000"},
	"aqu-sz":   {"count", "average disk queue size over the row", "delta(io_queued_ms) / (seconds * 1000)"},
	"util%":    {"%", "share of time the disk was busy", "delta(io_time_ms) / (seconds * 1000) * 100"},

	// network
	"activeConn":     {"count", "connections running an operation", "serverStatus.connections.active"},
	"idleConn":       {"count", "open connections not running an operation", "max(serverStatus.connections.current - active, 0)"},
	"totalCreated/s": {"/s", "new connections per second", "rate(serverStatus.connections.totalCreated)"},
	"queuedConn":     {"count", "connections queued during establishment", "serverStatus.connections.queuedForEstablishment"},
	"rejConn/s":      {"/s", "rejected connections per second", "rate(serverStatus.connections.rejected)"},
	"dnsSlow/s":      {"/s", "slow DNS resolutions per second", "rate(serverStatus.network.numSlowDNSOperations)"},
	"tlsSlow/s":      {"/s", "slow TLS handshakes per second", "rate(serverStatus.network.numSlowSSLOperations)"},
	"netTimeout/s":   {"/s", "connection network timeouts per second", "rate(serverStatus.metrics.operation.numConnectionNetworkTimeouts)"},

	// wiredTiger
	"wtCache%":     {"%", "WiredTiger cache used, percent of the configured size", "cache.bytes currently in the cache / cache.maximum bytes configured * 100"},
	"dirty%":       {"%", "dirty bytes, percent of the configured cache size", "cache.tracked dirty bytes in the cache / cache.maximum bytes configured * 100"},
	"cacheMB":      {"MB", "WiredTiger cache used", "cache.bytes currently in the cache / 1024 / 1024"},
	"dirtyMB":      {"MB", "dirty bytes in the WiredTiger cache", "cache.tracked dirty bytes in the cache / 1024 / 1024"},
	"updatesMB":    {"MB", "update bytes in the WiredTiger cache", "first of cache.bytes belonging to the updates in the cache / tracked bytes ... / bytes allocated for updates"},
	"wtRdMB/s":     {"MiB/s", "bytes read into the WiredTiger cache", "rate(cache.bytes read into cache) / 1024 / 1024"},
	"wtWrMB/s":     {"MiB/s", "bytes written from the WiredTiger cache", "rate(cache.bytes written from cache) / 1024 / 1024"},
	"evict/s":      {"/s", "pages evicted per second, all threads", "rate(pages evicted by eviction server + by application threads); newer: modified + unmodified pages evicted"},
	"appEvict/s":   {"/s", "pages evicted by application threads per second", "rate(pages evicted by application threads); newer: page evict attempts - failures by application threads"},
	"evictWalks/s": {"/s", "eviction walks started per second", "rate(eviction walks started from root of tree + from saved location in tree)"},
	"evictBusy/s":  {"/s", "pages selected for eviction that could not be evicted", "rate(sum of cache.pages selected for eviction unable to be evicted*)"},
	"ckptMS":       {"ms", "duration of the most recent checkpoint", "checkpoint.most recent time (msecs); older: transaction checkpoint most recent time (msecs)"},
	"ckptPages/s":  {"/s", "checkpoint pages written per second", "first rate of transaction checkpoint pages written / checkpoint-cleanup.pages written / checkpoint.number of pages caused to be reconciled"},
	"rdTkt":        {"count", "read tickets available; from 7.0 the total is dynamic", "concurrentTransactions.read.available or queues.execution.read.available"},
	"wrTkt":        {"count", "write tickets available; from 7.0 the total is dynamic", "concurrentTransactions.write.available or queues.execution.write.available"},
	"hsInsert/s":   {"/s", "history store inserts per second", "rate(history store table insert calls)"},
	"hsRead/s":     {"/s", "history store reads per second", "rate(history store table read calls / reads)"},
	"hsWriteMB/s":  {"MiB/s", "bytes written to the history store", "rate(cache.bytes written from cache into history store) / 1024 / 1024"},
	"appEvictMS/s": {"ms/s", "time application threads spent evicting; 1000 = one thread busy all second (8.0; 7.0 has no such counter)", "rate(cache.application thread time evicting (usecs)) / 1000"},
	"appReadMS/s":  {"ms/s", "time application threads spent reading pages from disk into the cache", "rate(cache.application threads page read from disk to cache time (usecs)) / 1000"},
	"logMB/s":      {"MiB/s", "journal (WiredTiger log) bytes written", "rate(log.log bytes written) / 1024 / 1024"},
	"logSyncMS":    {"ms", "average journal sync duration", "delta(log.log sync time duration (usecs)) / delta(log.log sync operations) / 1000"},

	// connPool (mongos)
	"clientConn":    {"count", "client connections in the router connection pool stats", "router.connPoolStats.numClientConnections"},
	"scopedConn":    {"count", "scoped connections", "router.connPoolStats.numAScopedConnections"},
	"poolInUse":     {"count", "pooled connections to shards in use", "router.connPoolStats.totalInUse"},
	"poolAvail":     {"count", "pooled connections to shards available", "router.connPoolStats.totalAvailable"},
	"poolCreate/s":  {"/s", "pooled connections created per second", "rate(router.connPoolStats.totalCreated)"},
	"poolRefresh/s": {"/s", "pooled connections refreshed per second", "rate(router.connPoolStats.totalRefreshed)"},
	"taskExec/s":    {"/s", "network tasks executed per second, all executors", "sum of rate(router.networkInterfaceStats.*.executed)"},
	"leased":        {"count", "pooled connections leased", "router.connPoolStats.totalLeased"},
	"refreshing":    {"count", "pooled connections being refreshed", "router.connPoolStats.totalRefreshing"},
	"helloAct":      {"count", "replica-set monitor hello calls in progress", "router.connPoolStats.replicaSetMonitor.hello.currentlyActive"},
	"ghaAct":        {"count", "getHostAndRefresh calls in progress (callers waiting for a host)", "router.connPoolStats.replicaSetMonitor.getHostAndRefresh.currentlyActive"},
	"rsmExec/s":     {"/s", "replica-set monitor executor tasks per second", "rate(router.networkInterfaceStats.ReplicaSetMonitor-TaskExecutor.executed)"},
	"shardExec/s":   {"/s", "shard registry executor tasks per second", "rate(router.networkInterfaceStats.ShardRegistry.executed)"},
}
