package webui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
	"ftdcstat/internal/render"
)

func TestBuildDatasetAveragesRowsIntoBuckets(t *testing.T) {
	metadata := model.NewMetadata()
	rows := []derive.Row{
		{
			Time: time.Date(2026, 6, 18, 12, 0, 5, 0, time.UTC),
			Values: map[string]any{
				"activeConn":      10.0,
				"totalCreated/s":  1.0,
				"queuedConn":      2.0,
				"processTextOnly": "kept",
			},
		},
		{
			Time: time.Date(2026, 6, 18, 12, 0, 40, 0, time.UTC),
			Values: map[string]any{
				"activeConn":     14.0,
				"totalCreated/s": 3.0,
				"queuedConn":     4.0,
			},
		},
		{
			Time: time.Date(2026, 6, 18, 12, 1, 5, 0, time.UTC),
			Values: map[string]any{
				"activeConn":     20.0,
				"totalCreated/s": 5.0,
				"queuedConn":     6.0,
			},
		},
	}

	dataset := BuildDataset(metadata, nil, rows, render.Options{View: "network", Verbose: true}, Options{
		View:         "network",
		Avg:          time.Minute,
		TimeLocation: time.UTC,
	})
	if !dataset.Metadata.Avg.Enabled || dataset.Metadata.Avg.Bucket != "1m0s" {
		t.Fatalf("avg=%#v", dataset.Metadata.Avg)
	}
	if len(dataset.Data.Rows) != 2 {
		t.Fatalf("rows=%d", len(dataset.Data.Rows))
	}
	first := dataset.Data.Rows[0]
	network := first.Sections["network"]
	if got := network["activeConn"]; got != 12.0 {
		t.Fatalf("activeConn=%v", got)
	}
	if got := network["totalCreated/s"]; got != 2.0 {
		t.Fatalf("totalCreated/s=%v", got)
	}
	if got := network["queuedConn"]; got != 3.0 {
		t.Fatalf("queuedConn=%v", got)
	}
}

func TestNewHandlerServesMetadataDataAndIndex(t *testing.T) {
	dataset := BuildDataset(model.NewMetadata(), nil, []derive.Row{{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"activeConn": 11.0,
		},
	}}, render.Options{View: "network"}, Options{
		View:         "network",
		TimeLocation: time.UTC,
	})

	server, err := NewServer(dataset)
	if err != nil {
		t.Fatal(err)
	}

	rootResp := serveTestRequest(t, server, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if !strings.Contains(rootResp, "200 OK") {
		t.Fatalf("root response=%q", rootResp)
	}
	if !strings.Contains(rootResp, `/style.css`) || !strings.Contains(rootResp, `/app.js`) {
		t.Fatalf("root body=%q", rootResp)
	}

	styleResp := serveTestRequest(t, server, "GET /style.css HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if !strings.Contains(styleResp, "200 OK") || !strings.Contains(styleResp, ".metadata-panel") {
		t.Fatalf("style response=%q", styleResp)
	}

	appResp := serveTestRequest(t, server, "GET /app.js HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if !strings.Contains(appResp, "200 OK") || !strings.Contains(appResp, "formatTooltipTime") {
		t.Fatalf("app response=%q", appResp)
	}

	var metadataResp MetadataResponse
	if err := json.Unmarshal(extractBody(t, serveTestRequest(t, server, "GET /api/metadata HTTP/1.1\r\nHost: localhost\r\n\r\n")), &metadataResp); err != nil {
		t.Fatal(err)
	}
	if metadataResp.View != "network" {
		t.Fatalf("view=%s", metadataResp.View)
	}
	if !strings.Contains(metadataResp.HeaderText, "network") || !strings.Contains(metadataResp.HeaderText, "maxConn") {
		t.Fatalf("headerText=%q", metadataResp.HeaderText)
	}

	var dataResp DataResponse
	if err := json.Unmarshal(extractBody(t, serveTestRequest(t, server, "GET /api/data HTTP/1.1\r\nHost: localhost\r\n\r\n")), &dataResp); err != nil {
		t.Fatal(err)
	}
	if len(dataResp.Rows) != 1 {
		t.Fatalf("rows=%d", len(dataResp.Rows))
	}
	if dataResp.Rows[0].Datetime != "2026-06-18T12:00:00Z" {
		t.Fatalf("datetime=%s", dataResp.Rows[0].Datetime)
	}
	if got := dataResp.Rows[0].Sections["network"]["activeConn"]; got != 11.0 {
		t.Fatalf("activeConn=%v", got)
	}
}

func TestBuildDatasetSplitsSystemDashboardSections(t *testing.T) {
	metadata := model.NewMetadata()
	row := derive.Row{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"user_cpu%":   11.0,
			"system_cpu%": 4.0,
			"iowait%":     2.0,
			"ctxt/s":      120.0,
			"residentMB":  512.0,
			"virtualMB":   2048.0,
			"swapIn/s":    0.5,
			"swapOut/s":   1.5,
			"r/s":         8.0,
			"w/s":         6.0,
			"rkB/s":       320.0,
			"wkB/s":       240.0,
			"awaitS":      0.110,
			"r_awaitS":    0.090,
			"w_awaitS":    0.140,
			"aqu-sz":      1.2,
			"util%":       77.0,
			"psiCpuSome%": 3.0,
			"psiMemSome%": 1.0,
			"psiMemFull%": 0.0,
			"psiIoSome%":  5.0,
			"psiIoFull%":  0.0,
		},
	}

	dataset := BuildDataset(metadata, nil, []derive.Row{row}, render.Options{View: "system", Verbose: true, Pressure: true}, Options{
		View:         "system",
		TimeLocation: time.UTC,
	})

	wantSections := []string{"system / CPU", "system / Memory", "system / Disks", "system / PSI"}
	if got := sectionNames(dataset.Metadata.Sections); strings.Join(got, "|") != strings.Join(wantSections, "|") {
		t.Fatalf("sections=%v want=%v", got, wantSections)
	}

	if got := MetricNames(dataset.Metadata.Sections[0]); strings.Join(got, "|") != strings.Join([]string{"ctxt/s", "iowait%", "system_cpu%", "user_cpu%"}, "|") {
		t.Fatalf("cpu metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[1]); strings.Join(got, "|") != strings.Join([]string{"residentMB", "swapIn/s", "swapOut/s", "virtualMB"}, "|") {
		t.Fatalf("memory metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[2]); strings.Join(got, "|") != strings.Join([]string{"aqu-sz", "awaitS", "r/s", "r_awaitS", "rkB/s", "util%", "w/s", "w_awaitS", "wkB/s"}, "|") {
		t.Fatalf("disk metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[3]); strings.Join(got, "|") != strings.Join([]string{"psiCpuSome%", "psiIoFull%", "psiIoSome%", "psiMemFull%", "psiMemSome%"}, "|") {
		t.Fatalf("psi metrics=%v", got)
	}

	first := dataset.Data.Rows[0].Sections
	if _, ok := first["system / CPU"]["ctxt/s"]; !ok {
		t.Fatalf("system / CPU missing ctxt/s: %#v", first["system / CPU"])
	}
	if _, ok := first["system / Memory"]["swapIn/s"]; !ok {
		t.Fatalf("system / Memory missing swapIn/s: %#v", first["system / Memory"])
	}
	if _, ok := first["system / Disks"]["rkB/s"]; !ok {
		t.Fatalf("system / Disks missing rkB/s: %#v", first["system / Disks"])
	}
	if _, ok := first["system / PSI"]["psiCpuSome%"]; !ok {
		t.Fatalf("system / PSI missing psiCpuSome%%: %#v", first["system / PSI"])
	}
}

func TestBuildDatasetHidesPSIDashboardWithoutPressureSection(t *testing.T) {
	metadata := model.NewMetadata()
	row := derive.Row{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"user_cpu%":   11.0,
			"system_cpu%": 4.0,
			"iowait%":     2.0,
			"residentMB":  512.0,
			"virtualMB":   2048.0,
			"r/s":         8.0,
			"w/s":         6.0,
			"awaitS":      0.110,
			"r_awaitS":    0.090,
			"w_awaitS":    0.140,
			"aqu-sz":      1.2,
			"util%":       77.0,
		},
	}

	dataset := BuildDataset(metadata, nil, []derive.Row{row}, render.Options{View: "system"}, Options{
		View:         "system",
		TimeLocation: time.UTC,
	})

	for _, name := range sectionNames(dataset.Metadata.Sections) {
		if name == "system / PSI" {
			t.Fatalf("unexpected PSI section without pressure data: %v", sectionNames(dataset.Metadata.Sections))
		}
	}
}

func TestBuildDatasetSplitsServerDashboardSections(t *testing.T) {
	metadata := model.NewMetadata()
	row := derive.Row{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"qTot":  3.0,
			"ins/s": 1.0,
			"qry/s": 2.0,
			"upd/s": 3.0,
			"del/s": 4.0,
			"getm/s": 5.0,
			"cmd/s":  6.0,
			"rLatS":  0.010,
			"wLatS":  0.020,
			"cLatS":  0.030,
		},
	}

	dataset := BuildDataset(metadata, nil, []derive.Row{row}, render.Options{View: "server"}, Options{
		View:         "server",
		TimeLocation: time.UTC,
	})

	wantSections := []string{"replication", "server / Commands", "server / Latency"}
	if got := sectionNames(dataset.Metadata.Sections); strings.Join(got, "|") != strings.Join(wantSections, "|") {
		t.Fatalf("sections=%v want=%v", got, wantSections)
	}
	if got := MetricNames(dataset.Metadata.Sections[1]); strings.Join(got, "|") != strings.Join([]string{"cmd/s", "del/s", "getm/s", "ins/s", "qTot", "qry/s", "upd/s"}, "|") {
		t.Fatalf("command metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[2]); strings.Join(got, "|") != strings.Join([]string{"cLatS", "rLatS", "wLatS"}, "|") {
		t.Fatalf("latency metrics=%v", got)
	}
	first := dataset.Data.Rows[0].Sections
	if _, ok := first["server / Commands"]["qTot"]; !ok {
		t.Fatalf("server / Commands missing qTot: %#v", first["server / Commands"])
	}
	if _, ok := first["server / Latency"]["rLatS"]; !ok {
		t.Fatalf("server / Latency missing rLatS: %#v", first["server / Latency"])
	}
	if _, ok := first["server / Commands"]["conn"]; ok {
		t.Fatalf("server / Commands should not include conn: %#v", first["server / Commands"])
	}
	if _, ok := first["server / Commands"]["rsState"]; ok {
		t.Fatalf("server / Commands should not include rsState: %#v", first["server / Commands"])
	}
}

func TestBuildDatasetSummaryKeepsServerSplitInPlace(t *testing.T) {
	metadata := model.NewMetadata()
	row := derive.Row{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"node1":       0.0,
			"majLagS":     0.0,
			"rsState":     "PRIMARY",
			"qTot":        3.0,
			"rLatS":       0.010,
			"wLatS":       0.020,
			"cLatS":       0.030,
			"activeConn":  11.0,
			"awaitS":      0.110,
			"util%":       77.0,
			"residentMB":  512.0,
			"wtCache%":    55.0,
			"dirty%":      4.0,
			"evict/s":     8.0,
			"appEvict/s":  6.0,
			"ckptMS":      50.0,
			"rdTkt":       32.0,
			"wrTkt":       32.0,
		},
	}

	dataset := BuildDataset(metadata, nil, []derive.Row{row}, render.Options{View: "summary"}, Options{
		View:         "summary",
		TimeLocation: time.UTC,
	})

	wantPrefix := []string{
		"replication",
		"server / Commands",
		"server / Latency",
		"network",
		"system / CPU",
		"system / Memory",
		"system / Disks",
		"wiredTiger / Tickets",
		"wiredTiger / Per-second rates",
		"wiredTiger / Checkpoint time",
		"wiredTiger / Percentages",
	}
	got := sectionNames(dataset.Metadata.Sections)
	if len(got) < len(wantPrefix) {
		t.Fatalf("summary sections too short: %v", got)
	}
	if strings.Join(got[:len(wantPrefix)], "|") != strings.Join(wantPrefix, "|") {
		t.Fatalf("summary sections=%v want prefix=%v", got, wantPrefix)
	}
}

func TestBuildDatasetSplitsWiredTigerDashboardSections(t *testing.T) {
	metadata := model.NewMetadata()
	row := derive.Row{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"rdTkt":        32.0,
			"wrTkt":        28.0,
			"wtRdMB/s":     4.5,
			"wtWrMB/s":     7.5,
			"evict/s":      8.0,
			"appEvict/s":   6.0,
			"evictWalks/s": 1.0,
			"evictBusy/s":  0.5,
			"ckptPages/s":  3.0,
			"hsInsert/s":   2.0,
			"hsRead/s":     1.5,
			"hsWriteMB/s":  0.8,
			"ckptMS":       50.0,
			"wtCache%":     55.0,
			"dirty%":       4.0,
			"cacheMB":      1024.0,
			"dirtyMB":      128.0,
			"updatesMB":    64.0,
		},
	}

	dataset := BuildDataset(metadata, nil, []derive.Row{row}, render.Options{View: "wt", Verbose: true}, Options{
		View:         "wt",
		TimeLocation: time.UTC,
	})

	wantSections := []string{
		"wiredTiger / Tickets",
		"wiredTiger / Per-second rates",
		"wiredTiger / Checkpoint time",
		"wiredTiger / Percentages",
		"wiredTiger / MiB",
	}
	if got := sectionNames(dataset.Metadata.Sections); strings.Join(got, "|") != strings.Join(wantSections, "|") {
		t.Fatalf("sections=%v want=%v", got, wantSections)
	}
	if got := MetricNames(dataset.Metadata.Sections[0]); strings.Join(got, "|") != strings.Join([]string{"rdTkt", "wrTkt"}, "|") {
		t.Fatalf("ticket metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[1]); strings.Join(got, "|") != strings.Join([]string{"appEvict/s", "ckptPages/s", "evict/s", "evictBusy/s", "evictWalks/s", "hsInsert/s", "hsRead/s", "hsWriteMB/s", "wtRdMB/s", "wtWrMB/s"}, "|") {
		t.Fatalf("rate metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[2]); strings.Join(got, "|") != strings.Join([]string{"ckptMS"}, "|") {
		t.Fatalf("checkpoint metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[3]); strings.Join(got, "|") != strings.Join([]string{"dirty%", "wtCache%"}, "|") {
		t.Fatalf("percentage metrics=%v", got)
	}
	if got := MetricNames(dataset.Metadata.Sections[4]); strings.Join(got, "|") != strings.Join([]string{"cacheMB", "dirtyMB", "updatesMB"}, "|") {
		t.Fatalf("mib metrics=%v", got)
	}

	first := dataset.Data.Rows[0].Sections
	if _, ok := first["wiredTiger / Tickets"]["rdTkt"]; !ok {
		t.Fatalf("wiredTiger / Tickets missing rdTkt: %#v", first["wiredTiger / Tickets"])
	}
	if _, ok := first["wiredTiger / Per-second rates"]["evictWalks/s"]; !ok {
		t.Fatalf("wiredTiger / Per-second rates missing evictWalks/s: %#v", first["wiredTiger / Per-second rates"])
	}
	if _, ok := first["wiredTiger / Checkpoint time"]["ckptMS"]; !ok {
		t.Fatalf("wiredTiger / Checkpoint time missing ckptMS: %#v", first["wiredTiger / Checkpoint time"])
	}
	if _, ok := first["wiredTiger / Percentages"]["wtCache%"]; !ok {
		t.Fatalf("wiredTiger / Percentages missing wtCache%%: %#v", first["wiredTiger / Percentages"])
	}
	if _, ok := first["wiredTiger / MiB"]["cacheMB"]; !ok {
		t.Fatalf("wiredTiger / MiB missing cacheMB: %#v", first["wiredTiger / MiB"])
	}
}

func sectionNames(sections []Section) []string {
	names := make([]string, 0, len(sections))
	for _, section := range sections {
		names = append(names, section.Name)
	}
	return names
}

func serveTestRequest(t *testing.T, server *Server, request string) string {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverFD := fds[0]
	clientFD := fds[1]
	clientFile := os.NewFile(uintptr(clientFD), "client-side")
	defer clientFile.Close()

	done := make(chan struct{})
	go func() {
		server.serveConn(serverFD)
		close(done)
	}()

	if _, err := clientFile.WriteString(request); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Shutdown(clientFD, syscall.SHUT_WR); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(bufio.NewReader(clientFile)); err != nil {
		t.Fatal(err)
	}
	<-done
	return buf.String()
}

func extractBody(t *testing.T, response string) []byte {
	t.Helper()
	parts := strings.SplitN(response, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid response: %q", response)
	}
	return []byte(parts[1])
}
