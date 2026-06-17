package ftdcstat

import (
	"os"
	"strings"
	"testing"
)

func TestREADMEDocumentsViewsColumnsAndFormulas(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"--view server|wt|system|network|repl|summary|all",
		"Default: `summary`.",
		"--interval",
		"--from",
		"--to",
		"--verbose",
		"--pressure",
		"network  Connection activity and network-establishment diagnostics",
		"replication | server | network | system | wiredTiger",
		"node1=localhost:27000",
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
		"`--view repl` is a compatibility alias that renders only the `replication`",
		"`--view summary` is intended for horizontal scrolling",
		"`replication.majLagS` contains the majority commit lag",
		"hbMs applyOps/s applyBufCnt applyBufMB",
		"wtCache% dirty% cacheMB dirtyMB updatesMB",
		"does not apply to `--view summary` or",
		"`--view network`",
		"activeConn idleConn totalCreated/s queuedConn rejConn/s dnsSlow/s tlsSlow/s netTimeout/s",
		"`summary` and `all` views also include the compact network section after",
		"The header always includes the `network` section and `maxConn`",
		"network\n  maxConn: <connections.current + connections.available from the first usable serverStatus sample>",
		"derived during metadata reading",
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
		"`getCmdLineOpts` prints the parsed startup config",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("README missing %q", want)
		}
	}
	if strings.Contains(text, "### `repl` View\n\n```text\nrsState") {
		t.Fatal("README should not document rsState as part of the repl section")
	}
}
