package derive

import "strings"

func RequiredPaths() (map[string]bool, []string) {
	return RequiredPathsFor("", false, false)
}

func ViewNeedsVerboseReplication(view string, verbose bool) bool {
	if !verbose {
		return false
	}
	switch view {
	case "repl":
		return true
	default:
		return false
	}
}

func ViewNeedsVerboseWiredTiger(view string, verbose bool) bool {
	return verbose && view == "wt"
}

func ViewNeedsVerboseSystem(view string, verbose bool) bool {
	return verbose && view == "system"
}

func ViewNeedsVerboseNetwork(view string, verbose bool) bool {
	return verbose && view == "network"
}

func ViewNeedsPressureSystem(view string, pressure bool) bool {
	return pressure && view == "system"
}

func RequiredPathsFor(view string, verbose, pressure bool) (map[string]bool, []string) {
	paths := map[string]bool{}
	for _, path := range exactRequiredPaths {
		paths[path] = true
	}
	if ViewNeedsVerboseReplication(view, verbose) {
		for _, path := range verboseReplicationPaths {
			paths[path] = true
		}
	}
	if ViewNeedsVerboseWiredTiger(view, verbose) {
		for _, path := range verboseWiredTigerPaths {
			paths[path] = true
		}
	}
	if ViewNeedsVerboseSystem(view, verbose) {
		for _, path := range verboseSystemPaths {
			paths[path] = true
		}
	}
	if ViewNeedsVerboseNetwork(view, verbose) {
		for _, path := range verboseNetworkPaths {
			paths[path] = true
		}
	}
	if ViewNeedsPressureSystem(view, pressure) {
		for _, path := range pressureSystemPaths {
			paths[path] = true
		}
	}
	return paths, append([]string(nil), requiredPrefixes...)
}

func Interesting(path string, exact map[string]bool, prefixes []string, verboseReplication bool) bool {
	if exact[path] {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if strings.HasPrefix(path, "replSetGetStatus.members.") {
		if verboseReplication && strings.HasSuffix(path, ".pingMs") {
			return true
		}
		return strings.HasSuffix(path, ".state") ||
			strings.HasSuffix(path, ".self") ||
			strings.HasSuffix(path, ".optimeDate") ||
			strings.HasSuffix(path, ".optime.ts.t")
	}
	return false
}

var verboseReplicationPaths = []string{
	"serverStatus.metrics.repl.apply.ops",
	"serverStatus.metrics.repl.buffer.apply.count",
	"serverStatus.metrics.repl.buffer.apply.sizeBytes",
}

var verboseWiredTigerPaths = []string{
	"serverStatus.wiredTiger.cache.bytes belonging to the updates in the cache",
	"serverStatus.wiredTiger.cache.tracked bytes belonging to the updates in the cache",
	"serverStatus.wiredTiger.cache.bytes allocated for updates",
	"serverStatus.wiredTiger.cache.eviction walks started from root of tree",
	"serverStatus.wiredTiger.cache.eviction walks started from saved location in tree",
	"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted",
	"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of active children on an internal page",
	"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of failure in reconciliation",
	"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of a cache overflow item",
	"serverStatus.wiredTiger.checkpoint.most recent duration for gathering all handles (usecs)",
	"serverStatus.wiredTiger.transaction.transaction checkpoint pages written",
	"serverStatus.wiredTiger.checkpoint-cleanup.pages written",
	"serverStatus.wiredTiger.checkpoint.number of pages caused to be reconciled",
	"serverStatus.wiredTiger.cache.history store table insert calls",
	"serverStatus.wiredTiger.history store.history store table insert calls",
	"serverStatus.wiredTiger.cache.history store table read calls",
	"serverStatus.wiredTiger.cache.history store table reads",
	"serverStatus.wiredTiger.history store.history store table read calls",
	"serverStatus.wiredTiger.cache.bytes written from cache into history store",
	"serverStatus.wiredTiger.history store.history store table bytes written",
}

var verboseSystemPaths = []string{
	"systemMetrics.cpu.ctxt",
	"systemMetrics.vmstat.pswpin",
	"systemMetrics.vmstat.pswpout",
}

var verboseNetworkPaths = []string{
	"serverStatus.connections.queuedForEstablishment",
	"serverStatus.connections.rejected",
	"serverStatus.network.numSlowDNSOperations",
	"serverStatus.network.numSlowSSLOperations",
	"serverStatus.metrics.operation.numConnectionNetworkTimeouts",
}

var pressureSystemPaths = []string{
	"systemMetrics.pressure.cpu.some.avg10",
	"systemMetrics.pressure.cpu.some.avg60",
	"systemMetrics.pressure.cpu.some.avg300",
	"systemMetrics.pressure.cpu.some.totalMicros",
	"systemMetrics.pressure.memory.some.avg10",
	"systemMetrics.pressure.memory.some.avg60",
	"systemMetrics.pressure.memory.some.avg300",
	"systemMetrics.pressure.memory.some.totalMicros",
	"systemMetrics.pressure.memory.full.avg10",
	"systemMetrics.pressure.memory.full.avg60",
	"systemMetrics.pressure.memory.full.avg300",
	"systemMetrics.pressure.memory.full.totalMicros",
	"systemMetrics.pressure.io.some.avg10",
	"systemMetrics.pressure.io.some.avg60",
	"systemMetrics.pressure.io.some.avg300",
	"systemMetrics.pressure.io.some.totalMicros",
	"systemMetrics.pressure.io.full.avg10",
	"systemMetrics.pressure.io.full.avg60",
	"systemMetrics.pressure.io.full.avg300",
	"systemMetrics.pressure.io.full.totalMicros",
}

var requiredPrefixes = []string{
	"systemMetrics.disks.",
}

var exactRequiredPaths = []string{
	"start",
	"end",
	"serverStatus.localTime",
	"serverStatus.process",
	"serverStatus.pid",
	"serverStatus.uptime",
	"serverStatus.uptimeMillis",
	"serverStatus.storageEngine.name",
	"serverStatus.mem.resident",
	"serverStatus.mem.virtual",
	"serverStatus.extra_info.system_time_us",
	"serverStatus.extra_info.user_time_us",
	"serverStatus.connections.current",
	"serverStatus.connections.available",
	"serverStatus.connections.active",
	"serverStatus.connections.totalCreated",
	"serverStatus.opcounters.insert",
	"serverStatus.opcounters.query",
	"serverStatus.opcounters.update",
	"serverStatus.opcounters.delete",
	"serverStatus.opcounters.getmore",
	"serverStatus.opcounters.command",
	"serverStatus.opcountersRepl.insert",
	"serverStatus.opcountersRepl.update",
	"serverStatus.opcountersRepl.delete",
	"serverStatus.opLatencies.reads.latency",
	"serverStatus.opLatencies.reads.ops",
	"serverStatus.opLatencies.writes.latency",
	"serverStatus.opLatencies.writes.ops",
	"serverStatus.opLatencies.commands.latency",
	"serverStatus.opLatencies.commands.ops",
	"serverStatus.opLatencies.transactions.latency",
	"serverStatus.opLatencies.transactions.ops",
	"serverStatus.globalLock.currentQueue.total",
	"serverStatus.repl.ismaster",
	"serverStatus.repl.isWritablePrimary",
	"serverStatus.repl.secondary",
	"serverStatus.repl.myState",
	"serverStatus.repl.lastWrite.lastWriteDate",
	"serverStatus.repl.lastWrite.majorityWriteDate",
	"serverStatus.wiredTiger.cache.bytes currently in the cache",
	"serverStatus.wiredTiger.cache.maximum bytes configured",
	"serverStatus.wiredTiger.cache.tracked dirty bytes in the cache",
	"serverStatus.wiredTiger.cache.pages read into cache",
	"serverStatus.wiredTiger.cache.pages written from cache",
	"serverStatus.wiredTiger.cache.bytes read into cache",
	"serverStatus.wiredTiger.cache.bytes written from cache",
	"serverStatus.wiredTiger.cache.pages evicted by eviction server",
	"serverStatus.wiredTiger.cache.pages evicted by application threads",
	"serverStatus.wiredTiger.cache.pages evicted because they exceeded the in-memory maximum",
	"serverStatus.wiredTiger.cache.unmodified pages evicted",
	"serverStatus.wiredTiger.cache.modified pages evicted",
	"serverStatus.wiredTiger.cache.eviction server candidate queue empty when topping up",
	"serverStatus.wiredTiger.cache.eviction server candidate queue not empty when topping up",
	"serverStatus.wiredTiger.cache.application threads page read from disk to cache count",
	"serverStatus.wiredTiger.transaction.transaction checkpoint most recent duration for gathering all handles (usecs)",
	"serverStatus.wiredTiger.concurrentTransactions.read.available",
	"serverStatus.wiredTiger.concurrentTransactions.write.available",
	"serverStatus.queues.execution.read.available",
	"serverStatus.queues.execution.write.available",
	"systemMetrics.cpu.user_ms",
	"systemMetrics.cpu.nice_ms",
	"systemMetrics.cpu.system_ms",
	"systemMetrics.cpu.idle_ms",
	"systemMetrics.cpu.iowait_ms",
	"systemMetrics.cpu.irq_ms",
	"systemMetrics.cpu.softirq_ms",
	"systemMetrics.cpu.steal_ms",
	"systemMetrics.cpu.procs_running",
	"systemMetrics.cpu.procs_blocked",
}
