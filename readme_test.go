package mongodbftdcstat

import (
	"os"
	"strings"
	"testing"

	"mongodb-ftdcstat/internal/render"
)

// TestREADMEDocumentsEveryColumn keeps the README column reference complete:
// every column --list-columns reports must appear in it.
func TestREADMEDocumentsEveryColumn(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, c := range render.ListColumns("") {
		if c.Column == "node1..nodeN" {
			continue
		}
		if !strings.Contains(text, c.Column) {
			t.Errorf("README does not mention column %q", c.Column)
		}
	}
}

func TestREADMEDocumentsViewsColumnsAndFormulas(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"--view server|wt|system|network|repl|summary|io",
		"Default: `summary`.",
		"`--interval` is no longer supported",
		"--avg",
		"The fetch columns tell a starved secondary from a slow one.",
		"The JSON document is written as a stream, one row at a time",
		"### `--columns COL,...`",
		"### `--list-columns`",
		"--columns replaces --view; using both is rejected",
		"`metadata` is a compact summary of the capture",
		"### `--stats` and `--findings`",
		"--stats and --findings cannot be combined with --avg, --web, or --tui",
		"--from",
		"--to",
		"--verbose",
		"--pressure",
		"--web",
		"--listen",
		"network  Connection activity and network-establishment diagnostics",
		"replication | server | network | system | io | wiredTiger",
		"primary.optimeDate - member.optimeDate",
		"lagS     replication header label",
		"rsState",
		"awaitS",
		"r_awaitS",
		"w_awaitS",
		"residentMB",
		"virtualMB",
		"user_cpu%",
		"system_cpu%",
		"rdTkt",
		"wrTkt",
		"sample time in UTC for FTDC sample rows",
		"normalized by the available CPU count",
		"delta(opLatencies.<type>.latency) / delta(opLatencies.<type>.ops) / 1000000",
		"`--view summary` is intended for horizontal scrolling",
		"`--view disk` is no longer supported; use `--view system`.",
		"`--view all` is no longer supported; use `--view summary`.",
		"`--view io` renders one top-level `io` section with one subgroup per device",
		"`--view io --json` emits nested per-device values at",
		"`replication.majLagS` contains the majority commit lag",
		"hbMs applyOps/s applyBufCnt applyBufMB replNetMB/s fetchBatchKB fetchMS fetchEmpty/s oplogServe/s oplogServeMS oplogMB oplogUsed% flowLagged flowRate flowWaitMS/s",
		"FTDC does not record the first and last oplog entry times",
		"hsWriteMB/s appEvictMS/s appReadMS/s logMB/s logSyncMS",
		"`wiredTiger / App thread time`",
		"wtCache% dirty% cacheMB dirtyMB updatesMB",
		"`--view network`",
		"activeConn idleConn totalCreated/s queuedConn rejConn/s dnsSlow/s tlsSlow/s netTimeout/s",
		"`summary` view also includes the compact network section after",
		"derived during metadata reading",
		"| Node 1       | localhost:27000 |",
		"`--view repl` renders only the `replication` section",
		"It is rejected for `--view summary` and `--view io`",
		"rdQMS, wrQMS    delta(queues.execution.<read|write>.normalPriority.totalTimeQueuedMicros)",
		"dbFree%      free space on the filesystem holding storage.dbPath, percent",
		"`<mount>` is the longest mount point in the capture that contains",
		"`Max connections` is `connections.current + connections.available`",
		"Command Line Options  parsed startup options, one per row",
		"`From` and `To` are the first and last rendered rows",
		"--- gap 1495s: rate baseline reset ---",
		"queuedConn    current queued connections during establishment",
		"netTimeout/s    = delta(serverStatus.metrics.operation.numConnectionNetworkTimeouts) / elapsed seconds",
		"`--pressure` is only supported for `--view system`",
		"rkB/s        disk read throughput in KiB/s, derived rate",
		"wkB/s        disk write throughput in KiB/s, derived rate",
		"ctxt/s       context switches per second, derived rate",
		"swapIn/s     swap-ins per second, derived rate",
		"`--verbose` and `--pressure` can be used together on `--view system`",
		"awaitS      average total disk wait in seconds, derived",
		"psiCpuSome%  CPU PSI pressure percent, derived",
		"aqu-sz` is average queue size over the row interval",
		"rate(systemMetrics.cpu.ctxt)",
		"prefer current avg10 from systemMetrics.pressure.<resource>.<scope>.avg10",
		"cacheMB       current WT cache bytes used",
		"hsWriteMB/s   history store bytes written per second",
		"WiredTiger column sources:",
		"serverStatus.wiredTiger.cache.bytes allocated for updates",
		"serverStatus.wiredTiger.checkpoint.number of pages caused to be reconciled",
		"serverStatus.wiredTiger.cache.history store table reads",
		"unavailable metrics render as `-`",
		"serverStatus.metrics.repl.apply.ops",
		"serverStatus.metrics.repl.buffer.apply.count",
		"`--json` cannot be combined with `--web` or `--tui`.",
		"`--avg` averages derived rows into fixed time buckets for both the terminal",
		"Valid bucket sizes are `1m` through `15m`.",
		"`--avg` rejects smaller or larger",
		"durations.",
		"Averaging: 5m buckets; datetime is bucket start; values are averaged per bucket.",
		"`/api/metadata`",
		"GET /api/data",
		"GET /app.js",
		"GET /style.css",
		"`headerText`",
		"`server / Commands`",
		"`server / Latency`",
		"`system / CPU`, `system / Processes`, `system / Memory`,",
		"`server / Admission`, `server / Query efficiency`",
		"`system / Disks`, `system / Disk space`, and `system / PSI`",
		"`wiredTiger / Tickets`",
		"`wiredTiger / Per-second rates`",
		"`wiredTiger / Checkpoint time`",
		"`wiredTiger / Percentages`",
		"`wiredTiger / MiB`",
		"Grafana-like",
		"exact UTC timestamp",
		"visible series values at that",
		"Large capture detected. Consider using --avg 5m or --from/--to for better browser performance.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("README missing %q", want)
		}
	}
	if !strings.Contains(text, "`--view repl` renders only the `replication` section, including per-node lag\ncolumns, `majLagS`, and `rsState`.") {
		t.Fatal("README should document rsState as part of the replication/repl section")
	}
	if strings.Contains(text, "It is also included with\n`--view server`") {
		t.Fatal("README should not document replication as part of --view server")
	}
	if strings.Contains(text, "`--web` starts a local HTTP server instead of printing the terminal table") {
		t.Fatal("README should document that --web still prints the terminal table")
	}
	if strings.Contains(text, "`--avg` is currently intended for `--web` mode") {
		t.Fatal("README should document avg for both table and web output")
	}
	if strings.Contains(text, "getCmdLineOpts argv") {
		t.Fatal("README should document parsed getCmdLineOpts config, not raw argv")
	}
	// The header used to be text sections; it is tables now.
	for _, stale := range []string{"webUI\n  url:", "metricsRange\n  start:", "network\n  maxConn:", "rsInfo\n  set=", "`<pre>` block", "compatibility alias"} {
		if strings.Contains(text, stale) {
			t.Fatalf("README still documents the old text header: %q", stale)
		}
	}
}
