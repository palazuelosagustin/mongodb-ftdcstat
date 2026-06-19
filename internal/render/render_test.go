package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
)

func testMetadata() model.Metadata {
	m := model.NewMetadata()
	ts := time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC)
	m.AddDocument(ts.Add(-time.Hour), "old-config", map[string]any{
		"replSetGetConfig": map[string]any{
			"config": map[string]any{
				"_id":             "rs0",
				"version":         1,
				"term":            1,
				"protocolVersion": 1,
				"members": []any{
					map[string]any{"_id": 0, "host": "h1:27017", "votes": 1, "priority": 1},
					map[string]any{"_id": 1, "host": "h2:27017", "votes": 1, "priority": 1},
				},
			},
		},
	})
	m.AddDocument(ts, "test", map[string]any{
		"buildInfo": map[string]any{
			"version": "8.0.0", "gitVersion": "abc", "allocator": "tcmalloc", "perconaFeatures": []any{"backup", "audit", "backup"},
		},
		"hostInfo": map[string]any{
			"system": map[string]any{
				"currentTime":                ts,
				"hostname":                   "h1",
				"cpuAddrSize":                64,
				"cpuArch":                    "x86_64",
				"numCores":                   8,
				"numCoresAvailableToProcess": 7,
				"numPhysicalCores":           4,
				"numCpuSockets":              1,
				"numNumaNodes":               2,
				"numaEnabled":                false,
				"memSizeMB":                  1024,
				"memLimitMB":                 2048,
			},
			"os": map[string]any{"name": "Linux", "version": "1"},
			"extra": map[string]any{
				"kernelVersion":     "6.8.0-test",
				"libcVersion":       "2.39",
				"maxOpenFiles":      1024,
				"pageSize":          4096,
				"numPages":          8216528,
				"overcommit_memory": "1",
				"thp_enabled":       "madvise",
				"thp_defrag":        "madvise",
				"thp_max_ptes_none": "511",
				"versionString":     "Linux version 6.8.0-test (builder) #1",
			},
		},
		"getCmdLineOpts": map[string]any{
			"argv": []any{"mongod", "--replSet", "rs0", "--dbpath", "/data/db", "--port", "27017", "--fork", "--setParameter", "wiredTigerConcurrentWriteTransactions=128"},
			"parsed": map[string]any{
				"storage":           map[string]any{"dbPath": "/data/db", "wiredTiger": map[string]any{"engineConfig": map[string]any{"cacheSizeGB": 1}}},
				"net":               map[string]any{"port": 27017},
				"replication":       map[string]any{"replSet": "rs0"},
				"processManagement": map[string]any{"fork": true},
				"setParameter":      map[string]any{"wiredTigerConcurrentWriteTransactions": 128},
			},
		},
		"getParameter": map[string]any{
			"transactionLifetimeLimitSeconds": 60,
		},
		"replSetGetConfig": map[string]any{
			"config": map[string]any{
				"_id":                                "rs0",
				"version":                            2,
				"term":                               1,
				"protocolVersion":                    1,
				"writeConcernMajorityJournalDefault": true,
				"members": []any{
					map[string]any{"_id": 0, "host": "h1:27017", "votes": 1, "priority": 2, "arbiterOnly": false, "hidden": false, "buildIndexes": true, "tags": map[string]any{"dc": "east"}},
					map[string]any{"_id": 1, "host": "h2:27017", "votes": 1, "priority": 1, "arbiterOnly": false, "hidden": false, "buildIndexes": true, "horizons": map[string]any{"external": "h2.example.net:27017"}},
				},
				"settings": map[string]any{"chainingAllowed": true, "electionTimeoutMillis": 10000, "getLastErrorDefaults": map[string]any{"w": "majority", "wtimeout": 0}},
			},
		},
		"replSetGetStatus": map[string]any{
			"set": "rs0",
			"members": []any{
				map[string]any{"name": "h1:27017", "stateStr": "PRIMARY"},
				map[string]any{"name": "h2:27017", "stateStr": "SECONDARY"},
			},
		},
		"serverStatus": map[string]any{
			"connections": map[string]any{
				"current":   9,
				"available": 400,
			},
		},
	})
	return m
}

func TestHeaderOmitsProcessRoleAndPrimary(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "buildInfo\n") {
		t.Fatalf("header should start with buildInfo section:\n%s", out)
	}
	if !strings.Contains(out, "  version=8.0.0 git=abc modules=- storage=- allocator=tcmalloc openssl=-\n  perconaFeatures=backup,audit\n") {
		t.Fatalf("perconaFeatures should be deduplicated and printed on its own line:\n%s", out)
	}
	if strings.Contains(out, "perconaFeatures=backup,audit,backup") || strings.Contains(out, "openssl=- perconaFeatures") {
		t.Fatalf("perconaFeatures should not be duplicated or printed inline:\n%s", out)
	}
	buildEnd := strings.Index(out, "\nrsInfo\n")
	if buildEnd < 0 {
		t.Fatalf("missing rsInfo section:\n%s", out)
	}
	if strings.Contains(out[:buildEnd], "replSet=") {
		t.Fatalf("buildInfo section should not contain replica set config:\n%s", out)
	}
	if strings.Contains(out, "process=mongod") || strings.Contains(out, " role=") || strings.Contains(out, " primary=") {
		t.Fatalf("static header has time-varying fields:\n%s", out)
	}
	for _, want := range []string{
		"rsInfo\n  set=rs0 members:\n",
		"    node1=h1:27017\n",
		"    node2=h2:27017\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing rsInfo member mapping %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\nhostInfo\n") || strings.Contains(out, "\nHost\n") {
		t.Fatalf("host section should be named hostInfo:\n%s", out)
	}
	if !strings.Contains(out, "network\n  maxConn: 409\n") {
		t.Fatalf("header should always include network maxConn:\n%s", out)
	}
	for _, want := range []string{
		"hostname=h1 os=Linux 1 kernel=6.8.0-test libc=2.39 arch=x86_64 cpuAddrSize=64 cores=8 availableCores=7 physicalCores=4 sockets=1 numaNodes=2 numaEnabled=false memoryMB=1024 memLimitMB=2048",
		"maxOpenFiles=1024 pageSize=4096 numPages=8216528 overcommit_memory=1",
		"thp_enabled=madvise thp_defrag=madvise thp_max_ptes_none=511",
		"versionString=Linux version 6.8.0-test (builder) #1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing hostInfo detail %q:\n%s", want, out)
		}
	}
	for _, removed := range []string{"currentTime", "kernelVersion=", "libcVersion=", "cpuArch=", "memSizeMB="} {
		if strings.Contains(out, removed) {
			t.Fatalf("hostInfo should not print duplicated or time-specific field %q:\n%s", removed, out)
		}
	}
	for _, removed := range []string{"rsConfig", "configsSeen=", "config._id=", "members[0].", "settings.", "replSetGetConfig=notFound", "nodes="} {
		if strings.Contains(out, removed) {
			t.Fatalf("rsInfo should not print full config detail %q:\n%s", removed, out)
		}
	}
}

func TestHeaderPrintsCmdLineOptsAndExplicitParametersOnly(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "getCmdLineOpts\n") {
		t.Fatalf("missing getCmdLineOpts section:\n%s", out)
	}
	for _, want := range []string{
		"  net.port=27017\n",
		"  processManagement.fork=true\n",
		"  replication.replSet=rs0\n",
		"  storage.dbPath=/data/db\n",
		"  storage.wiredTiger.engineConfig.cacheSizeGB=1\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing parsed getCmdLineOpts item %q:\n%s", want, out)
		}
	}
	for _, removed := range []string{
		"argv=mongod",
		"/usr/bin/mongod",
		"--replSet",
		"--dbpath",
		"--port",
		"--fork",
		"--setParameter",
	} {
		if strings.Contains(out, removed) {
			t.Fatalf("getCmdLineOpts should not print raw argv token %q:\n%s", removed, out)
		}
	}
	for _, removed := range []string{"  storage=", "  net=", "  replication=", "  processManagement=", "  setParameter="} {
		if strings.Contains(out, removed) {
			t.Fatalf("getCmdLineOpts printed parsed option %q:\n%s", removed, out)
		}
	}
	if !strings.Contains(out, "wtCache=1") {
		t.Fatalf("missing wt cache:\n%s", out)
	}
	if !strings.Contains(out, " wiredTigerConcurrentWriteTransactions=128\n") {
		t.Fatalf("missing explicit setParameter item in Parameters:\n%s", out)
	}
	if strings.Contains(out, "transactionLifetime") {
		t.Fatalf("default getParameter leaked into Parameters:\n%s", out)
	}
}

func TestHeaderPrintsWebUISectionBeforeTableWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{
		View:   "server",
		WebURL: "http://127.0.0.1:55508",
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "webUI\n  url: http://127.0.0.1:55508\n") {
		t.Fatalf("missing webUI header section:\n%s", out)
	}
	networkIdx := strings.Index(out, "\nnetwork\n  maxConn: 409\n")
	webIdx := strings.Index(out, "\nwebUI\n  url: http://127.0.0.1:55508\n")
	labelLine, _ := firstTableHeader(out)
	tableIdx := strings.Index(out, labelLine)
	if networkIdx < 0 || webIdx < 0 || tableIdx < 0 {
		t.Fatalf("expected network, webUI, and table header sections:\n%s", out)
	}
	if !(networkIdx < webIdx && webIdx < tableIdx) {
		t.Fatalf("webUI header should appear after network and before the metrics table:\n%s", out)
	}
}

func TestHeaderOmitsWebUISectionWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\nwebUI\n") {
		t.Fatalf("unexpected webUI header without --web:\n%s", buf.String())
	}
}

func TestStreamingRendererMatchesRenderOutput(t *testing.T) {
	rows := make([]derive.Row, 51)
	for i := range rows {
		rows[i] = testRow(i)
	}
	rows[0].ProcessMarker = "--- mongod process: pid=10 start=2026-06-04T18:59:00-03:00 ---"
	rows[10].Marker = "gap 120s: rate baseline reset"

	for _, opts := range []Options{
		{View: "summary"},
		{View: "system", Verbose: true, Pressure: true},
		{View: "network", Verbose: true},
	} {
		var want, got bytes.Buffer
		if err := Render(&want, testMetadata(), nil, rows, opts); err != nil {
			t.Fatal(err)
		}
		streamer, err := NewStreamingRenderer(&got, testMetadata(), opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if err := streamer.RenderRow(row); err != nil {
				t.Fatal(err)
			}
		}
		if err := streamer.Close(); err != nil {
			t.Fatal(err)
		}
		if got.String() != want.String() {
			t.Fatalf("streaming output mismatch for %#v\nwant:\n%s\ngot:\n%s", opts, want.String(), got.String())
		}
	}
}

func TestStreamingRendererRejectsJSON(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewStreamingRenderer(&buf, testMetadata(), Options{View: "summary", JSON: true})
	if err == nil {
		t.Fatal("expected JSON streaming constructor error")
	}
}

func TestRSInfoFallbackUsesReplSetGetStatus(t *testing.T) {
	m := model.NewMetadata()
	ts := time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC)
	m.AddDocument(ts, "test", map[string]any{
		"buildInfo": map[string]any{"version": "8.0.0"},
		"replSetGetStatus": map[string]any{
			"set": "rs0",
			"members": []any{
				map[string]any{"name": "h1:27017"},
				map[string]any{"name": "h2:27017"},
			},
		},
	})
	var buf bytes.Buffer
	if err := Render(&buf, m, nil, []derive.Row{testRow(0)}, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"rsInfo\n  set=rs0 members:\n",
		"    node1=h1:27017\n",
		"    node2=h2:27017\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing replSetGetStatus fallback member mapping %q:\n%s", want, out)
		}
	}
	for _, removed := range []string{"rsConfig", "replSetGetConfig=notFound", "fallback.replSetGetStatus", "config._id=", "members[0].", "settings.", "nodes="} {
		if strings.Contains(out, removed) {
			t.Fatalf("rsInfo fallback should only print name and nodes, found %q:\n%s", removed, out)
		}
	}
}

func TestProcessMarkerBeforeFirstMetricLineAndRestartMarker(t *testing.T) {
	rows := []derive.Row{
		testRow(0),
		testRow(1),
	}
	rows[0].ProcessMarker = "--- mongod process: pid=10 start=2026-06-04T18:59:00-03:00 ---"
	rows[1].ProcessMarker = "--- mongod restart detected: pid=11 start=2026-06-04T19:01:00-03:00 ---"
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	processIdx := strings.Index(out, "mongod process")
	firstRowIdx := strings.Index(out, formatRowTime(testRow(0).Time, time.UTC))
	restartIdx := strings.Index(out, "mongod restart detected: pid=11")
	if processIdx < 0 || firstRowIdx < 0 || processIdx > firstRowIdx {
		t.Fatalf("process marker not before first metric row:\n%s", out)
	}
	if restartIdx < 0 {
		t.Fatalf("missing restart marker:\n%s", out)
	}
}

func TestSummaryViewIsSingleWideTableAndRepeatsHeader(t *testing.T) {
	rows := make([]derive.Row, 51)
	for i := range rows {
		rows[i] = testRow(i)
	}
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "summary"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "[summary]") || strings.Contains(out, "[disk]") {
		t.Fatalf("summary view should not be stacked sections:\n%s", out)
	}
	if strings.Contains(out, "[server]") || strings.Contains(out, "[wiredTiger]") || strings.Contains(out, "[system]") ||
		strings.Contains(out, "----") {
		t.Fatalf("summary view should use compact section labels, not old banners:\n%s", out)
	}
	assertSummaryViewHeaders(t, out, 2)
	if !strings.Contains(out, "wtCache%") || !strings.Contains(out, "user_cpu%") || !strings.Contains(out, "awaitS") || !strings.Contains(out, "activeConn") {
		t.Fatalf("summary view missing grouped columns:\n%s", out)
	}
	labelLine, headerLine := firstTableHeader(out)
	assertSectionOrder(t, labelLine, []string{"replication", "server", "network", "system", "wiredTiger"})
	if !strings.Contains(headerLine, "lagS") || !strings.Contains(headerLine, "node1") || !strings.Contains(headerLine, "node2") {
		t.Fatalf("replication columns should use generic node labels:\n%s", out)
	}
	if strings.Contains(headerLine, "h1:27017") || strings.Contains(headerLine, "h2:27017") {
		t.Fatalf("replication columns should not use hostnames:\n%s", out)
	}
	if !strings.Contains(headerLine, "node1 node2 majLagS rsState") {
		t.Fatalf("replication section should end with majLagS rsState:\n%s", out)
	}
	if !strings.Contains(headerLine, "activeConn idleConn totalCreated/s") {
		t.Fatalf("network section should follow server columns:\n%s", out)
	}
	if strings.Contains(headerLine, " conn ") || strings.Contains(headerLine, " conn|") || strings.Contains(headerLine, "| conn ") {
		t.Fatalf("server section should not include conn:\n%s", out)
	}
	for _, want := range []string{"rLatS", "wLatS", "cLatS"} {
		if !strings.Contains(headerLine, want) {
			t.Fatalf("server section should include %s:\n%s", want, out)
		}
	}
	for _, old := range []string{" rLat ", " wLat ", " cLat "} {
		if strings.Contains(headerLine, old) {
			t.Fatalf("server section should not include old latency column %q:\n%s", old, out)
		}
	}
	if !strings.Contains(headerLine, "datetime") || !strings.Contains(headerLine, "lagS node1 node2 majLagS rsState") || !strings.Contains(headerLine, "qTot") {
		t.Fatalf("replication should include lagS, node lags, majLagS, rsState and server should start with qTot:\n%s", out)
	}
	if !strings.Contains(headerLine, "activeConn idleConn totalCreated/s") {
		t.Fatalf("summary view should include network columns after server:\n%s", out)
	}
	if !strings.Contains(out, "PRIMARY") {
		t.Fatalf("output should keep per-row rsState values:\n%s", out)
	}
}

func TestNonVerboseOutputUnchanged(t *testing.T) {
	rows := []derive.Row{testRow(0), testRow(1)}
	var replPlain, summaryPlain bytes.Buffer
	if err := Render(&replPlain, testMetadata(), nil, rows, Options{View: "repl"}); err != nil {
		t.Fatal(err)
	}
	if err := Render(&summaryPlain, testMetadata(), nil, rows, Options{View: "summary"}); err != nil {
		t.Fatal(err)
	}
	_, replHeader := firstTableHeader(replPlain.String())
	_, summaryHeader := firstTableHeader(summaryPlain.String())
	if strings.Contains(replHeader, "hbMs") || strings.Contains(replHeader, "applyOps/s") {
		t.Fatalf("non-verbose repl should not include verbose columns:\n%s", replPlain.String())
	}
	if strings.Contains(summaryHeader, "hbMs") || strings.Contains(summaryHeader, "applyBufMB") {
		t.Fatalf("non-verbose summary should not include verbose columns:\n%s", summaryPlain.String())
	}
	if strings.Contains(summaryHeader, "queuedConn") {
		t.Fatalf("non-verbose summary should not include verbose network columns:\n%s", summaryPlain.String())
	}
}

func TestVerboseReplViewIncludesReplicationMetrics(t *testing.T) {
	row := verboseReplicationRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "repl", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	for _, col := range []string{"hbMs", "applyOps/s", "applyBufCnt", "applyBufMB"} {
		if !strings.Contains(headerLine, col) {
			t.Fatalf("repl verbose header missing %s:\n%s", col, out)
		}
	}
	for _, forbidden := range []string{"term", "applyB/s", "appBufCnt", "appBufMB"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("repl verbose output should not contain %q:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(headerLine, "lagS node1 node2 majLagS rsState hbMs applyOps/s applyBufCnt applyBufMB") {
		t.Fatalf("unexpected verbose repl header order:\n%s", headerLine)
	}
}

func TestSummaryViewIgnoresVerboseFlag(t *testing.T) {
	row := verboseSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "summary", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	for _, col := range []string{"hbMs", "applyOps/s", "applyBufCnt", "applyBufMB", "rkB/s", "wkB/s", "ctxt/s", "swapIn/s", "psiCpuSome%"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("summary should ignore --verbose and exclude %s:\n%s", col, out)
		}
	}
}

func TestSummaryViewIncludesNetworkSection(t *testing.T) {
	row := networkRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "summary"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	assertSectionOrder(t, labelLine, []string{"replication", "server", "network", "system", "wiredTiger"})
	if !strings.Contains(headerLine, "activeConn idleConn totalCreated/s") {
		t.Fatalf("summary should include network columns after server:\n%s", out)
	}
}

func TestAllViewKeepsCompactSystemColumns(t *testing.T) {
	row := verboseSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "all"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	for _, col := range []string{"r/s", "w/s", "awaitS", "user_cpu%", "residentMB", "virtualMB"} {
		if !strings.Contains(headerLine, col) {
			t.Fatalf("all view header missing %s:\n%s", col, out)
		}
	}
	for _, col := range []string{"rkB/s", "wkB/s", "ctxt/s", "swapIn/s", "swapOut/s", "psiCpuSome%"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("all view should keep compact system columns and exclude %s:\n%s", col, out)
		}
	}
}

func TestVerboseSystemViewExcludesReplicationMetrics(t *testing.T) {
	row := verboseReplicationRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, col := range []string{"hbMs", "applyOps/s", "applyBufCnt", "applyBufMB", "majLagS", "lagS"} {
		if strings.Contains(out, col) {
			t.Fatalf("system verbose should not include replication column %q:\n%s", col, out)
		}
	}
}

func TestSystemViewKeepsCompactDefaultColumns(t *testing.T) {
	row := verboseSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "system") {
		t.Fatalf("system output missing section label:\n%s", out)
	}
	for _, col := range []string{"r/s", "w/s", "awaitS", "r_awaitS", "w_awaitS", "aqu-sz", "util%", "user_cpu%", "system_cpu%", "iowait%", "residentMB", "virtualMB"} {
		if !strings.Contains(headerLine, col) {
			t.Fatalf("system default header missing %s:\n%s", col, out)
		}
	}
	for _, col := range []string{"rkB/s", "wkB/s", "ctxt/s", "swapIn/s", "swapOut/s", "psiCpuSome%"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("system default header should not include optional column %s:\n%s", col, out)
		}
	}
}

func TestVerboseSystemViewIncludesVerboseColumns(t *testing.T) {
	row := verboseSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "system") {
		t.Fatalf("system verbose output missing section label:\n%s", out)
	}
	headerText := normalizedTableLine(headerLine)
	wantOrder := strings.Join(systemColumns(true), " ")
	if !strings.Contains(headerText, wantOrder) {
		t.Fatalf("unexpected system verbose header order:\n%s", headerLine)
	}
	for _, col := range []string{"idle_cpu%", "memFreeMB", "memAvailMB", "cachedMB", "swapUsedMB", "psiCpuSome%"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("system verbose should not include %s:\n%s", col, out)
		}
	}
	values := tableRowValues(t, out)
	for key, want := range map[string]string{
		"rkB/s":     "0.0",
		"wkB/s":     "4.0",
		"ctxt/s":    "12.0",
		"swapIn/s":  "6.0",
		"swapOut/s": "7.0",
		"awaitS":    "0.001",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s=%q want %q\n%s", key, got, want, out)
		}
	}
}

func TestSystemPressureViewIncludesPressureColumns(t *testing.T) {
	row := pressureSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Pressure: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "system") || !strings.Contains(labelLine, "pressure") {
		t.Fatalf("system pressure output missing section labels:\n%s", out)
	}
	headerText := normalizedTableLine(headerLine)
	wantOrder := strings.Join(append(systemColumns(false), pressureColumns()...), " ")
	if !strings.Contains(headerText, wantOrder) {
		t.Fatalf("unexpected system pressure header order:\n%s", headerLine)
	}
	values := tableRowValues(t, out)
	for key, want := range map[string]string{
		"psiCpuSome%": "16.0",
		"psiMemSome%": "17.0",
		"psiMemFull%": "18.0",
		"psiIoSome%":  "19.0",
		"psiIoFull%":  "20.0",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s=%q want %q\n%s", key, got, want, out)
		}
	}
}

func TestSystemVerbosePressureViewIncludesBothColumnSets(t *testing.T) {
	row := verboseSystemRow(0)
	row.Values["psiCpuSome%"] = float64(16)
	row.Values["psiMemSome%"] = float64(17)
	row.Values["psiMemFull%"] = float64(18)
	row.Values["psiIoSome%"] = float64(19)
	row.Values["psiIoFull%"] = float64(20)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Verbose: true, Pressure: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "system") || !strings.Contains(labelLine, "pressure") {
		t.Fatalf("system verbose+pressure output missing section labels:\n%s", out)
	}
	headerText := normalizedTableLine(headerLine)
	wantOrder := strings.Join(append(systemColumns(true), pressureColumns()...), " ")
	if !strings.Contains(headerText, wantOrder) {
		t.Fatalf("unexpected system verbose+pressure header order:\n%s", headerLine)
	}
}

func TestSummaryViewKeepsCompactSystemColumnsWhenVerbose(t *testing.T) {
	row := verboseSystemRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "summary", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	for _, col := range []string{"rkB/s", "wkB/s", "ctxt/s", "swapIn/s", "swapOut/s", "psiCpuSome%"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("summary view should keep compact system columns and exclude %s:\n%s", col, out)
		}
	}
}

func TestVerboseSystemMissingMetricsRenderDash(t *testing.T) {
	row := testRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	values := tableRowValues(t, buf.String())
	for _, col := range []string{"ctxt/s", "swapIn/s", "swapOut/s"} {
		if got := values[col]; got != "-" {
			t.Fatalf("%s=%q want '-'\n%s", col, got, buf.String())
		}
	}
}

func TestPressureSystemMissingMetricsRenderDash(t *testing.T) {
	row := testRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "system", Pressure: true}); err != nil {
		t.Fatal(err)
	}
	values := tableRowValues(t, buf.String())
	for _, col := range []string{"psiCpuSome%", "psiMemSome%", "psiMemFull%", "psiIoSome%", "psiIoFull%"} {
		if got := values[col]; got != "-" {
			t.Fatalf("%s=%q want '-'\n%s", col, got, buf.String())
		}
	}
}

func TestSystemHeaderRepeatsWithVerbosePressureFlags(t *testing.T) {
	rows := make([]derive.Row, 51)
	for i := range rows {
		rows[i] = verboseSystemRow(i)
		rows[i].Values["psiCpuSome%"] = float64(16)
	}
	for _, opts := range []Options{
		{View: "system", Verbose: true},
		{View: "system", Pressure: true},
		{View: "system", Verbose: true, Pressure: true},
	} {
		var buf bytes.Buffer
		if err := Render(&buf, testMetadata(), nil, rows, opts); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if got := strings.Count(out, "datetime"); got != 2 {
			t.Fatalf("header count=%d want 2 for %#v:\n%s", got, opts, out)
		}
		if strings.Contains(out, "----") || strings.Contains(out, "[system]") {
			t.Fatalf("should not restore old banner separators for %#v:\n%s", opts, out)
		}
	}
}

func TestNetworkViewIncludesHeaderMetadataAndDefaultColumns(t *testing.T) {
	m := testMetadata()
	ts := time.Date(2026, 6, 4, 18, 0, 0, 0, time.UTC)
	m.AddDocument(ts, "serverStatus-early", map[string]any{
		"serverStatus": map[string]any{
			"connections": map[string]any{
				"current":   12,
				"available": 65524,
			},
		},
	})
	var buf bytes.Buffer
	if err := Render(&buf, m, nil, []derive.Row{networkRow(0)}, Options{View: "network"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "network\n  maxConn: 65536\n") {
		t.Fatalf("network header metadata missing:\n%s", out)
	}
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "network") {
		t.Fatalf("network output missing section label:\n%s", out)
	}
	headerText := normalizedTableLine(headerLine)
	if !strings.Contains(headerText, strings.Join(networkColumns(false), " ")) {
		t.Fatalf("unexpected network header order:\n%s", headerLine)
	}
	for _, col := range []string{"queuedConn", "rejConn/s", "dnsSlow/s", "tlsSlow/s", "netTimeout/s"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("default network header should not include %s:\n%s", col, out)
		}
	}
	values := tableRowValues(t, out)
	for key, want := range map[string]string{
		"activeConn":     "12",
		"idleConn":       "184",
		"totalCreated/s": "0.5",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s=%q want %q\n%s", key, got, want, out)
		}
	}
}

func TestVerboseNetworkViewIncludesVerboseColumns(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{networkRow(0)}, Options{View: "network", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	headerText := normalizedTableLine(headerLine)
	if !strings.Contains(headerText, strings.Join(networkColumns(true), " ")) {
		t.Fatalf("unexpected verbose network header order:\n%s", headerLine)
	}
	values := tableRowValues(t, out)
	for key, want := range map[string]string{
		"queuedConn":   "0",
		"rejConn/s":    "0.0",
		"dnsSlow/s":    "0.0",
		"tlsSlow/s":    "0.0",
		"netTimeout/s": "0.0",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s=%q want %q\n%s", key, got, want, out)
		}
	}
}

func TestVerboseNetworkMissingMetricsRenderDash(t *testing.T) {
	row := derive.Row{
		Time:   time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC),
		Values: map[string]any{"activeConn": float64(5)},
	}
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "network", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	values := tableRowValues(t, buf.String())
	for _, col := range []string{"idleConn", "totalCreated/s", "queuedConn", "rejConn/s", "dnsSlow/s", "tlsSlow/s", "netTimeout/s"} {
		if got := values[col]; got != "-" {
			t.Fatalf("%s=%q want '-'\n%s", col, got, buf.String())
		}
	}
}

func TestNetworkHeaderPrintsDashWhenMaxConnUnavailable(t *testing.T) {
	m := model.NewMetadata()
	var buf bytes.Buffer
	if err := Render(&buf, m, nil, []derive.Row{networkRow(0)}, Options{View: "network"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "network\n  maxConn: -\n") {
		t.Fatalf("expected maxConn dash:\n%s", buf.String())
	}
}

func TestWiredTigerViewKeepsCompactDefaultColumns(t *testing.T) {
	row := verboseWiredTigerRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "wt"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "wiredTiger") {
		t.Fatalf("wt default output missing wiredTiger section label:\n%s", out)
	}
	for _, col := range []string{"wtCache%", "dirty%", "wtRdMB/s", "wtWrMB/s", "evict/s", "appEvict/s", "ckptMS", "rdTkt", "wrTkt"} {
		if !strings.Contains(headerLine, col) {
			t.Fatalf("wt default header missing %s:\n%s", col, out)
		}
	}
	for _, col := range []string{"cacheMB", "dirtyMB", "updatesMB", "evictWalks/s", "evictBusy/s", "ckptPages/s", "hsInsert/s", "hsRead/s", "hsWriteMB/s"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("wt default header should not include verbose column %s:\n%s", col, out)
		}
	}
}

func TestVerboseWiredTigerViewIncludesVerboseColumns(t *testing.T) {
	row := verboseWiredTigerRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "wt", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	if !strings.Contains(labelLine, "wiredTiger") {
		t.Fatalf("wt verbose output missing wiredTiger section label:\n%s", out)
	}
	headerText := normalizedTableLine(headerLine)
	wantOrder := "wtCache% dirty% cacheMB dirtyMB updatesMB wtRdMB/s wtWrMB/s evict/s appEvict/s evictWalks/s evictBusy/s ckptMS ckptPages/s rdTkt wrTkt hsInsert/s hsRead/s hsWriteMB/s"
	if !strings.Contains(headerText, wantOrder) {
		t.Fatalf("unexpected wt verbose header order:\n%s", headerLine)
	}
	if !strings.Contains(headerText, "rdTkt wrTkt") {
		t.Fatalf("wt verbose header should keep rdTkt/wrTkt names:\n%s", headerLine)
	}
	if strings.Contains(headerLine, "readT") || strings.Contains(headerLine, "writeT") {
		t.Fatalf("wt verbose header should not rename tickets:\n%s", headerLine)
	}
	values := tableRowValues(t, out)
	for key, want := range map[string]string{
		"cacheMB":      "512",
		"dirtyMB":      "10",
		"updatesMB":    "20",
		"evictWalks/s": "3.0",
		"evictBusy/s":  "4.0",
		"ckptPages/s":  "5.0",
		"hsInsert/s":   "6.0",
		"hsRead/s":     "7.0",
		"hsWriteMB/s":  "8.5",
		"wtCache%":     "50.0",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s=%q want %q\n%s", key, got, want, out)
		}
	}
}

func TestSummaryViewKeepsCompactWiredTigerColumnsWhenVerbose(t *testing.T) {
	row := verboseWiredTigerRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "summary", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	_, headerLine := firstTableHeader(out)
	for _, col := range []string{"cacheMB", "dirtyMB", "updatesMB", "evictWalks/s", "evictBusy/s", "ckptPages/s", "hsInsert/s", "hsRead/s", "hsWriteMB/s"} {
		if strings.Contains(headerLine, col) {
			t.Fatalf("summary view should keep compact wt columns and exclude %s:\n%s", col, out)
		}
	}
}

func TestVerboseWiredTigerMissingMetricsRenderDash(t *testing.T) {
	row := testRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "wt", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	values := tableRowValues(t, buf.String())
	for _, col := range []string{"cacheMB", "dirtyMB", "updatesMB", "evictWalks/s", "evictBusy/s", "ckptPages/s", "hsInsert/s", "hsRead/s", "hsWriteMB/s"} {
		if got := values[col]; got != "-" {
			t.Fatalf("%s=%q want '-'\n%s", col, got, buf.String())
		}
	}
}

func TestVerboseWiredTigerHeaderRepeatsAndKeepsCompactSeparators(t *testing.T) {
	rows := make([]derive.Row, 51)
	for i := range rows {
		rows[i] = verboseWiredTigerRow(i)
	}
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "wt", Verbose: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if got := strings.Count(out, "datetime"); got != 2 {
		t.Fatalf("verbose wt header count=%d want 2:\n%s", got, out)
	}
	if strings.Contains(out, "----") || strings.Contains(out, "[wiredTiger]") {
		t.Fatalf("verbose wt should not restore old banner separators:\n%s", out)
	}
}

func TestVerboseJSONIncludesReplicationMetrics(t *testing.T) {
	row := verboseReplicationRow(0)
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "repl", JSON: true, Verbose: true}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rows := payload["rows"].([]any)
	gotRow := rows[0].(map[string]any)
	replication := gotRow["replication"].(map[string]any)
	for _, key := range []string{"hbMs", "applyOps/s", "applyBufCnt", "applyBufMB"} {
		if _, ok := replication[key]; !ok {
			t.Fatalf("replication missing %q: %#v", key, replication)
		}
	}
}

func TestServerViewIncludesReplicationAndServerSections(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	assertSectionOrder(t, labelLine, []string{"server"})
	if strings.Count(labelLine, "|") != 1 || !strings.Contains(headerLine, "datetime") || !strings.Contains(headerLine, "qTot") {
		t.Fatalf("server view should render datetime | server:\n%s", out)
	}
	for _, forbidden := range []string{"lagS", "node1", "node2", "majLagS", "rsState"} {
		if strings.Contains(headerLine, forbidden) {
			t.Fatalf("server view should not include replication column %s:\n%s", forbidden, out)
		}
	}
	if strings.Contains(headerLine, " conn ") || strings.Contains(headerLine, " conn|") || strings.Contains(headerLine, "| conn ") {
		t.Fatalf("server view should not include conn:\n%s", out)
	}
}

func TestReplicationLagSIsHeaderOnlyNotRepeatedPerRow(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "2026-06-04T19:00:00Z") && strings.Contains(line, "| lagS ") {
			t.Fatalf("lagS should be header-only, not repeated in data rows:\n%s", out)
		}
	}
}

func TestReplViewRendersSingleReplicationSection(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "repl"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "rsState") {
		t.Fatalf("repl view should include rsState in replication output:\n%s", out)
	}
	if strings.Contains(out, " conn ") || strings.Contains(out, " conn|") || strings.Contains(out, "| conn ") {
		t.Fatalf("repl view should not include conn:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	var headerLine string
	for _, line := range lines {
		if strings.Contains(line, "datetime") {
			headerLine = line
			break
		}
	}
	if headerLine == "" {
		t.Fatalf("repl view missing column header:\n%s", out)
	}
	if !strings.Contains(headerLine, "lagS node1 node2 majLagS rsState") {
		t.Fatalf("repl view should render lagS node1 node2 majLagS rsState columns:\n%s", out)
	}
	values := replicationDataValues(t, out)
	if values["majLagS"] != "0.0" {
		t.Fatalf("repl view majLagS=%q", values["majLagS"])
	}
	if values["rsState"] != "PRIMARY" {
		t.Fatalf("repl view rsState=%q", values["rsState"])
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "repl" {
			t.Fatalf("repl view should not render a section named repl:\n%s", out)
		}
	}
}

func TestTableHeaderDoesNotRepeatBeforeFiftyRows(t *testing.T) {
	rows := make([]derive.Row, 50)
	for i := range rows {
		rows[i] = testRow(i)
	}
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "summary"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if got := strings.Count(out, "datetime"); got != 1 {
		t.Fatalf("header repeated before 50 rendered rows, got %d headers:\n%s", got, out)
	}
	assertSummaryViewHeaders(t, out, 1)
}

func assertSummaryViewHeaders(t *testing.T, out string, want int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	found := 0
	for i, line := range lines {
		if !strings.Contains(line, "datetime") {
			continue
		}
		found++
		if strings.Count(line, "|") < 2 {
			t.Fatalf("column header should separate groups with pipes:\n%s", out)
		}
		if i == 0 {
			t.Fatalf("missing section-label row before column header:\n%s", out)
		}
		labelLine := lines[i-1]
		if !strings.Contains(labelLine, "replication") || !strings.Contains(labelLine, "server") || !strings.Contains(labelLine, "wiredTiger") || !strings.Contains(labelLine, "system") {
			t.Fatalf("missing section labels before column header:\n%s", out)
		}
		if strings.Count(labelLine, "|") < 4 {
			t.Fatalf("section-label row should separate groups with pipes:\n%s", out)
		}
		if strings.Contains(labelLine, "-") {
			t.Fatalf("section-label row should not use dashed banners:\n%s", out)
		}
	}
	if found != want {
		t.Fatalf("got %d table headers, want %d:\n%s", found, want, out)
	}
}

func firstTableHeader(out string) (string, string) {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "datetime") && i > 0 {
			return lines[i-1], line
		}
	}
	return "", ""
}

func tableRowValues(t *testing.T, out string) map[string]string {
	t.Helper()
	_, headerLine := firstTableHeader(out)
	if headerLine == "" {
		t.Fatalf("missing table header:\n%s", out)
	}
	header := strings.Fields(strings.ReplaceAll(headerLine, "|", ""))
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "2026-06-04T19:00:00Z") {
			continue
		}
		fields := strings.Fields(strings.ReplaceAll(line, "|", ""))
		if len(fields) != len(header) {
			t.Fatalf("field count mismatch header=%v row=%v\n%s", header, fields, out)
		}
		values := map[string]string{}
		for i, col := range header {
			values[col] = fields[i]
		}
		return values
	}
	t.Fatalf("missing first data row:\n%s", out)
	return nil
}

func normalizedTableLine(line string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(line, "|", "")), " ")
}

func assertSectionOrder(t *testing.T, line string, labels []string) {
	t.Helper()
	last := -1
	for _, label := range labels {
		idx := strings.Index(line, label)
		if idx < 0 {
			t.Fatalf("missing section %s in label line %q", label, line)
		}
		if idx <= last {
			t.Fatalf("section labels out of order in %q", line)
		}
		last = idx
	}
}

func TestSystemViewIncludesCoreDefaultColumns(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "system"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, col := range []string{"user_cpu%", "system_cpu%", "iowait%", "residentMB", "virtualMB", "awaitS"} {
		if !strings.Contains(out, col) {
			t.Fatalf("missing %s in system output:\n%s", col, out)
		}
	}
}

func TestZeroVsMissingDisplay(t *testing.T) {
	row := testRow(0)
	row.Values["ins/s"] = float64(0)
	delete(row.Values, "qry/s")
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, " 0 ") {
		t.Fatalf("zero integer value not shown as 0:\n%s", out)
	}
	if !strings.Contains(out, "0.0") {
		t.Fatalf("zero rate value not shown as 0.0:\n%s", out)
	}
	if !strings.Contains(out, " - ") {
		t.Fatalf("missing value not shown as -:\n%s", out)
	}
}

func TestNumericFormattingByColumnType(t *testing.T) {
	cases := []struct {
		key  string
		val  any
		want string
	}{
		{"conn", float64(0), "0"},
		{"residentMB", float64(100), "100"},
		{"ins/s", float64(0), "0.0"},
		{"user_cpu%", float64(0), "0.0"},
		{"ctxt/s", float64(1234.5), "1234.5"},
		{"awaitS", float64(0), "0.000"},
		{"rLatS", float64(0), "0.000"},
		{"node1", float64(0), "0.0"},
		{"node2", float64(12), "12.0"},
		{"swapIn/s", float64(6), "6.0"},
		{"psiCpuSome%", float64(16), "16.0"},
		{"hbMs", float64(15.5), "15.5"},
		{"applyOps/s", float64(100), "100.0"},
		{"applyBufCnt", float64(42), "42"},
		{"applyBufMB", float64(2), "2.0"},
		{"cacheMB", float64(512), "512"},
		{"dirtyMB", float64(10), "10"},
		{"updatesMB", float64(20), "20"},
		{"hsWriteMB/s", float64(8.5), "8.5"},
		{"evictWalks/s", float64(3), "3.0"},
		{"wtCache%", float64(50), "50.0"},
		{"qry/s", nil, "-"},
	}
	for _, tc := range cases {
		if got := format(tc.val, tc.key); got != tc.want {
			t.Fatalf("format(%s)=%q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestSummaryAndReplViewsMatchReplicationValues(t *testing.T) {
	row := testRow(0)
	delete(row.Values, "node2")
	var replBuf, summaryBuf bytes.Buffer
	if err := Render(&replBuf, testMetadata(), nil, []derive.Row{row}, Options{View: "repl"}); err != nil {
		t.Fatal(err)
	}
	if err := Render(&summaryBuf, testMetadata(), nil, []derive.Row{row}, Options{View: "summary"}); err != nil {
		t.Fatal(err)
	}
	replValues := replicationDataValues(t, replBuf.String())
	summaryValues := replicationDataValues(t, summaryBuf.String())
	if !reflect.DeepEqual(replValues, summaryValues) {
		t.Fatalf("replication values mismatch:\nrepl=%#v\nsummary=%#v", replValues, summaryValues)
	}
}

func replicationDataValues(t *testing.T, out string) map[string]string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "2026-06-04T19:00:00Z") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			t.Fatalf("expected replication | ... data row:\n%s", out)
		}
		fields := strings.Fields(strings.TrimSpace(parts[1]))
		if len(fields) < 3 {
			t.Fatalf("expected node lag, majLagS, and rsState values:\n%s", out)
		}
		values := map[string]string{
			"majLagS": fields[len(fields)-2],
			"rsState": fields[len(fields)-1],
		}
		for i, field := range fields[:len(fields)-2] {
			values[fmt.Sprintf("node%d", i+1)] = field
		}
		return values
	}
	t.Fatalf("missing data row in output:\n%s", out)
	return nil
}

func TestJSONSummaryViewIncludesMajLagSInReplication(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "summary", JSON: true}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rows := payload["rows"].([]any)
	gotRow := rows[0].(map[string]any)
	replication := gotRow["replication"].(map[string]any)
	if replication["majLagS"] != float64(0) {
		t.Fatalf("replication.majLagS=%#v", replication["majLagS"])
	}
	if _, ok := gotRow["repl"]; ok {
		t.Fatalf("summary JSON should not contain repl section: %#v", gotRow)
	}
	server := gotRow["server"].(map[string]any)
	if replication["rsState"] != "PRIMARY" {
		t.Fatalf("replication.rsState=%#v", replication["rsState"])
	}
	if _, ok := server["rsState"]; ok {
		t.Fatalf("server should not contain rsState: %#v", server)
	}
	if _, ok := server["conn"]; ok {
		t.Fatalf("server should not contain conn: %#v", server)
	}
}

func TestJSONReplViewUsesReplicationSectionOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "repl", JSON: true}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rows := payload["rows"].([]any)
	gotRow := rows[0].(map[string]any)
	if _, ok := gotRow["repl"]; ok {
		t.Fatalf("repl JSON should not contain repl section: %#v", gotRow)
	}
	replication := gotRow["replication"].(map[string]any)
	if replication["majLagS"] != float64(0) {
		t.Fatalf("replication.majLagS=%#v", replication["majLagS"])
	}
	if replication["rsState"] != "PRIMARY" {
		t.Fatalf("replication.rsState=%#v", replication["rsState"])
	}
	if _, ok := gotRow["server"]; ok {
		t.Fatalf("repl JSON should not contain server section: %#v", gotRow)
	}
}

func TestJSONIncludesRSInfoMappingAndReplicationGroup(t *testing.T) {
	row := testRow(0)
	delete(row.Values, "node2")
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{row}, Options{View: "server", JSON: true}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rsInfo := payload["rsInfo"].(map[string]any)
	if rsInfo["set"] != "rs0" {
		t.Fatalf("rsInfo.set=%v", rsInfo["set"])
	}
	members := rsInfo["members"].(map[string]any)
	if members["node1"] != "h1:27017" || members["node2"] != "h2:27017" {
		t.Fatalf("bad rsInfo members: %#v", members)
	}
	rows := payload["rows"].([]any)
	gotRow := rows[0].(map[string]any)
	if _, ok := gotRow["replication"]; ok {
		t.Fatalf("server JSON should not contain replication section: %#v", gotRow)
	}
	server := gotRow["server"].(map[string]any)
	if _, ok := server["rsState"]; ok {
		t.Fatalf("server should not contain rsState: %#v", server)
	}
	if _, ok := server["conn"]; ok {
		t.Fatalf("server should not contain conn: %#v", server)
	}
}

func testRow(i int) derive.Row {
	ts := time.Date(2026, 6, 4, 19, i, 0, 0, time.UTC)
	return derive.Row{
		Time: ts,
		Values: map[string]any{
			"rsState":          "PRIMARY",
			"node1":            float64(0),
			"node2":            float64(1),
			"majLagS":          float64(0),
			"conn":             float64(5),
			"qTot":             float64(0),
			"ins/s":            float64(0),
			"qry/s":            float64(1),
			"upd/s":            float64(0),
			"del/s":            float64(0),
			"getm/s":           float64(0),
			"cmd/s":            float64(2),
			"rLatS":            float64(0.2),
			"wLatS":            float64(0.3),
			"cLatS":            float64(0.4),
			"activeConn":       float64(8),
			"availConn":        float64(379),
			"queuedConn":       float64(2),
			"rejectedConn/s":   float64(0),
			"totalConn/s":      float64(1),
			"netInMB/s":        float64(1.2),
			"netOutMB/s":       float64(2.3),
			"physInMB/s":       float64(0.7),
			"physOutMB/s":      float64(0.8),
			"requests/s":       float64(12),
			"scan/s":           float64(3),
			"scanObj/s":        float64(4),
			"collScan/s":       float64(5),
			"docRet/s":         float64(6),
			"docIns/s":         float64(7),
			"docUpd/s":         float64(8),
			"docDel/s":         float64(9),
			"writeConf/s":      float64(10),
			"openCursor":       float64(11),
			"pinnedCursor":     float64(1),
			"timedOutCursor/s": float64(0),
			"assertUser/s":     float64(0),
			"assertWarn/s":     float64(0),
			"replNetMB/s":      float64(0.3),
			"replOps/s":        float64(12),
			"applyOps/s":       float64(11.8),
			"applyBatchOps":    float64(16),
			"applyBatchMS":     float64(4.2),
			"writeBatchOps":    float64(8),
			"writeBatchMS":     float64(2.1),
			"bufferCnt":        float64(0),
			"bufferMB":         float64(0),
			"syncChanges/s":    float64(0),
			"flowLagged":       float64(0),
			"flowLagged/s":     float64(0),
			"flowLockMicros/s": float64(0),
			"flowTarget":       float64(1000),
			"wtCache%":         float64(50),
			"dirty%":           float64(1),
			"wtRdMB/s":         float64(0),
			"wtWrMB/s":         float64(0),
			"evict/s":          float64(0),
			"appEvict/s":       float64(0),
			"ckptMS":           float64(0),
			"rdTkt":            float64(128),
			"wrTkt":            float64(128),
			"wtCacheMB":        float64(512),
			"wtCacheMaxMB":     float64(1024),
			"wtDirtyMB":        float64(10),
			"pagesIn/s":        float64(1),
			"pagesOut/s":       float64(2),
			"evictFail/s":      float64(0),
			"evictBlocked/s":   float64(0),
			"appEvictUsec/s":   float64(5),
			"hsCacheMB":        float64(6),
			"hsFullUpd/s":      float64(7),
			"hsRevMod/s":       float64(8),
			"txnBeg/s":         float64(9),
			"txnCommit/s":      float64(10),
			"txnRollback/s":    float64(11),
			"updConflict/s":    float64(12),
			"prepTxn":          float64(0),
			"r/s":              float64(0),
			"w/s":              float64(1),
			"rkB/s":            float64(0),
			"wkB/s":            float64(4),
			"awaitS":           float64(0.001),
			"r_awaitS":         float64(0.002),
			"w_awaitS":         float64(0.003),
			"aqu-sz":           float64(0),
			"util%":            float64(1),
			"user_cpu%":        float64(2),
			"system_cpu%":      float64(3),
			"iowait%":          float64(0),
			"residentMB":       float64(100),
			"virtualMB":        float64(200),
			"threads":          float64(80),
			"ctxVol/s":         float64(3),
			"ctxInv/s":         float64(1),
			"pageRecl/s":       float64(4),
			"pageFault/s":      float64(5),
			"rssMaxMB":         float64(150),
			"psiCpuSome/s":     float64(0),
			"psiCpuFull/s":     float64(0),
			"psiIoSome/s":      float64(0),
			"psiIoFull/s":      float64(0),
			"psiMemSome/s":     float64(0),
			"psiMemFull/s":     float64(0),
		},
	}
}

func verboseReplicationRow(i int) derive.Row {
	row := testRow(i)
	row.Values["hbMs"] = float64(15)
	row.Values["applyOps/s"] = float64(100)
	row.Values["applyBufCnt"] = float64(42)
	row.Values["applyBufMB"] = float64(2)
	return row
}

func verboseWiredTigerRow(i int) derive.Row {
	row := testRow(i)
	row.Values["cacheMB"] = float64(512)
	row.Values["dirtyMB"] = float64(10)
	row.Values["updatesMB"] = float64(20)
	row.Values["evictWalks/s"] = float64(3)
	row.Values["evictBusy/s"] = float64(4)
	row.Values["ckptPages/s"] = float64(5)
	row.Values["hsInsert/s"] = float64(6)
	row.Values["hsRead/s"] = float64(7)
	row.Values["hsWriteMB/s"] = float64(8.5)
	return row
}

func verboseSystemRow(i int) derive.Row {
	row := testRow(i)
	row.Values["rkB/s"] = float64(0)
	row.Values["wkB/s"] = float64(4)
	row.Values["ctxt/s"] = float64(12)
	row.Values["swapIn/s"] = float64(6)
	row.Values["swapOut/s"] = float64(7)
	return row
}

func pressureSystemRow(i int) derive.Row {
	row := testRow(i)
	row.Values["psiCpuSome%"] = float64(16)
	row.Values["psiMemSome%"] = float64(17)
	row.Values["psiMemFull%"] = float64(18)
	row.Values["psiIoSome%"] = float64(19)
	row.Values["psiIoFull%"] = float64(20)
	return row
}

func networkRow(i int) derive.Row {
	row := testRow(i)
	row.Values["activeConn"] = float64(12)
	row.Values["idleConn"] = float64(184)
	row.Values["totalCreated/s"] = float64(0.5)
	row.Values["queuedConn"] = float64(0)
	row.Values["rejConn/s"] = float64(0)
	row.Values["dnsSlow/s"] = float64(0)
	row.Values["tlsSlow/s"] = float64(0)
	row.Values["netTimeout/s"] = float64(0)
	return row
}
