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
		"--view server|wt|system|repl|all",
		"--interval",
		"--from",
		"--to",
		"replication | server | system | wiredTiger",
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
		"`replication.majLagS` contains the majority commit lag",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("README missing %q", want)
		}
	}
	if strings.Contains(text, "### `repl` View\n\n```text\nrsState") {
		t.Fatal("README should not document rsState as part of the repl section")
	}
}
