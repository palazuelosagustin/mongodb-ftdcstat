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
	if !strings.Contains(out, "getCmdLineOpts") || !strings.Contains(out, "  argv=mongod\n") {
		t.Fatalf("missing getCmdLineOpts argv section:\n%s", out)
	}
	for _, want := range []string{
		"          --replSet rs0\n",
		"          --dbpath /data/db\n",
		"          --port 27017\n",
		"          --fork\n",
		"          --setParameter wiredTigerConcurrentWriteTransactions=128\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing multiline argv item %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "argv=mongod --") {
		t.Fatalf("argv should not be rendered as a single line:\n%s", out)
	}
	for _, removed := range []string{"  storage=", "  net=", "  replication=", "  processManagement=", "  setParameter="} {
		if strings.Contains(out, removed) {
			t.Fatalf("getCmdLineOpts printed parsed option %q:\n%s", removed, out)
		}
	}
	if !strings.Contains(out, "wtCache=1") {
		t.Fatalf("missing wt cache:\n%s", out)
	}
	if strings.Contains(out, "transactionLifetime") {
		t.Fatalf("default getParameter leaked into Parameters:\n%s", out)
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

func TestAllViewIsSingleWideTableAndRepeatsHeader(t *testing.T) {
	rows := make([]derive.Row, 51)
	for i := range rows {
		rows[i] = testRow(i)
	}
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "all"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "[summary]") || strings.Contains(out, "[disk]") {
		t.Fatalf("all view should not be stacked sections:\n%s", out)
	}
	if strings.Contains(out, "[server]") || strings.Contains(out, "[wiredTiger]") || strings.Contains(out, "[system]") ||
		strings.Contains(out, "----") {
		t.Fatalf("all view should use compact section labels, not old banners:\n%s", out)
	}
	assertAllViewHeaders(t, out, 2)
	if !strings.Contains(out, "wtCache%") || !strings.Contains(out, "user_cpu%") || !strings.Contains(out, "awaitS") {
		t.Fatalf("all view missing grouped columns:\n%s", out)
	}
	labelLine, headerLine := firstTableHeader(out)
	assertSectionOrder(t, labelLine, []string{"replication", "server", "system", "wiredTiger"})
	if !strings.Contains(headerLine, "lagS") || !strings.Contains(headerLine, "node1") || !strings.Contains(headerLine, "node2") {
		t.Fatalf("replication columns should use generic node labels:\n%s", out)
	}
	if strings.Contains(headerLine, "h1:27017") || strings.Contains(headerLine, "h2:27017") {
		t.Fatalf("replication columns should not use hostnames:\n%s", out)
	}
	if !strings.Contains(headerLine, "rsState conn qTot") {
		t.Fatalf("server section should start with rsState conn qTot:\n%s", out)
	}
	if strings.Contains(headerLine, "node1 node2 rsState") {
		t.Fatalf("rsState should not be in replication:\n%s", out)
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
	if !strings.Contains(headerLine, "datetime") || !strings.Contains(headerLine, "lagS node1 node2 majLagS") || !strings.Contains(headerLine, "rsState conn qTot") {
		t.Fatalf("replication should include lagS, node lags, majLagS and server should start with rsState:\n%s", out)
	}
	if !strings.Contains(out, "PRIMARY") {
		t.Fatalf("output should keep per-row rsState values:\n%s", out)
	}
}

func TestServerViewIncludesReplicationAndServerSections(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "server"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	labelLine, headerLine := firstTableHeader(out)
	assertSectionOrder(t, labelLine, []string{"replication", "server"})
	if strings.Count(labelLine, "|") != 2 || !strings.Contains(headerLine, "datetime") || !strings.Contains(headerLine, "lagS node1 node2") || !strings.Contains(headerLine, "rsState conn qTot") {
		t.Fatalf("server view should render datetime | replication | server:\n%s", out)
	}
	if strings.Contains(headerLine, "node1 node2 rsState") {
		t.Fatalf("rsState should not appear in replication:\n%s", out)
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
	if strings.Contains(out, " rsState") || strings.Contains(out, "rsState conn") {
		t.Fatalf("repl view should not include server columns:\n%s", out)
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
	if !strings.Contains(headerLine, "lagS node1 node2 majLagS") {
		t.Fatalf("repl view should render lagS node1 node2 majLagS columns:\n%s", out)
	}
	values := replicationDataValues(t, out)
	if values["majLagS"] != "0.0" {
		t.Fatalf("repl view majLagS=%q", values["majLagS"])
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
	if err := Render(&buf, testMetadata(), nil, rows, Options{View: "all"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if got := strings.Count(out, "datetime"); got != 1 {
		t.Fatalf("header repeated before 50 rendered rows, got %d headers:\n%s", got, out)
	}
	assertAllViewHeaders(t, out, 1)
}

func assertAllViewHeaders(t *testing.T, out string, want int) {
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

func TestSystemViewKeepsCompactDefaultColumns(t *testing.T) {
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
		{"awaitS", float64(0), "0.000"},
		{"rLatS", float64(0), "0.000"},
		{"node1", float64(0), "0.0"},
		{"node2", float64(12), "12.0"},
		{"qry/s", nil, "-"},
	}
	for _, tc := range cases {
		if got := format(tc.val, tc.key); got != tc.want {
			t.Fatalf("format(%s)=%q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestAllAndReplViewsMatchReplicationValues(t *testing.T) {
	row := testRow(0)
	delete(row.Values, "node2")
	var replBuf, allBuf bytes.Buffer
	if err := Render(&replBuf, testMetadata(), nil, []derive.Row{row}, Options{View: "repl"}); err != nil {
		t.Fatal(err)
	}
	if err := Render(&allBuf, testMetadata(), nil, []derive.Row{row}, Options{View: "all"}); err != nil {
		t.Fatal(err)
	}
	replValues := replicationDataValues(t, replBuf.String())
	allValues := replicationDataValues(t, allBuf.String())
	if !reflect.DeepEqual(replValues, allValues) {
		t.Fatalf("replication values mismatch:\nrepl=%#v\nall=%#v", replValues, allValues)
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
		if len(fields) < 2 {
			t.Fatalf("expected node lag and majLagS values:\n%s", out)
		}
		values := map[string]string{
			"majLagS": fields[len(fields)-1],
		}
		for i, field := range fields[:len(fields)-1] {
			values[fmt.Sprintf("node%d", i+1)] = field
		}
		return values
	}
	t.Fatalf("missing data row in output:\n%s", out)
	return nil
}

func TestJSONAllViewIncludesMajLagSInReplication(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, testMetadata(), nil, []derive.Row{testRow(0)}, Options{View: "all", JSON: true}); err != nil {
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
		t.Fatalf("all JSON should not contain repl section: %#v", gotRow)
	}
	server := gotRow["server"].(map[string]any)
	if server["rsState"] != "PRIMARY" {
		t.Fatalf("server.rsState=%#v", server["rsState"])
	}
	if _, ok := replication["rsState"]; ok {
		t.Fatalf("replication should not contain rsState: %#v", replication)
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
	replication := gotRow["replication"].(map[string]any)
	lagS := replication["lagS"].(map[string]any)
	if lagS["node1"] != float64(0) {
		t.Fatalf("node1 lag=%#v", lagS["node1"])
	}
	if _, ok := lagS["node2"]; !ok || lagS["node2"] != nil {
		t.Fatalf("node2 missing lag should be JSON null: %#v", lagS)
	}
	if replication["majLagS"] != float64(0) {
		t.Fatalf("replication.majLagS=%#v", replication["majLagS"])
	}
	if _, ok := replication["rsState"]; ok {
		t.Fatalf("replication should not contain rsState: %#v", replication)
	}
	server := gotRow["server"].(map[string]any)
	if server["rsState"] != "PRIMARY" {
		t.Fatalf("server.rsState=%#v", server["rsState"])
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
