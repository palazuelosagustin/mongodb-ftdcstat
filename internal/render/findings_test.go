package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
)

func findingsTestMetadata(process string) model.Metadata {
	m := model.NewMetadata()
	m.AddDocument(time.Unix(0, 0), "metadata", map[string]any{"serverStatus": map[string]any{"process": process}})
	return m
}

func TestFindingsCollectorEvents(t *testing.T) {
	start := time.Date(2026, 6, 4, 22, 0, 0, 0, time.UTC)
	c := NewFindingsCollector(findingsTestMetadata(model.ProcessKindMongod))

	first := statsTestRow(100, map[string]any{"rsState": "PRIMARY"})
	first.Process = &derive.ProcessInfo{PID: 10, Start: start}
	c.Add(first)
	c.Add(statsTestRow(101, map[string]any{"rsState": "PRIMARY"}))
	c.Add(statsTestRow(102, map[string]any{"rsState": "SECONDARY"}))
	c.Add(statsTestRow(103, map[string]any{})) // no state: not a change
	c.Add(statsTestRow(104, map[string]any{"rsState": "SECONDARY"}))
	gap := statsTestRow(300, map[string]any{"rsState": "SECONDARY"})
	gap.GapSeconds = 196
	c.Add(gap)
	restart := statsTestRow(301, map[string]any{"rsState": "STARTUP2"})
	restart.Process = &derive.ProcessInfo{Restart: true, PID: 11}
	c.Add(restart)

	events := c.Report().Events
	var got []string
	for _, e := range events {
		got = append(got, e.TimeStr+" "+e.Type+" "+findingsDetail(e))
	}
	want := []string{
		"1970-01-01T00:01:40Z process mongod pid=10 start=2026-06-04T22:00:00Z",
		"1970-01-01T00:01:42Z rsState PRIMARY -> SECONDARY",
		"1970-01-01T00:05:00Z gap 3m16s without samples, rate baseline reset",
		"1970-01-01T00:05:01Z restart mongod pid=11 start=-",
		"1970-01-01T00:05:01Z rsState SECONDARY -> STARTUP2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFindingsCollectorMongosProcessKind(t *testing.T) {
	c := NewFindingsCollector(findingsTestMetadata(model.ProcessKindMongos))
	row := statsTestRow(1, map[string]any{})
	row.Process = &derive.ProcessInfo{PID: 7}
	c.Add(row)
	if events := c.Report().Events; len(events) != 1 || events[0].Process != model.ProcessKindMongos {
		t.Fatalf("events=%+v", events)
	}
}

func TestRenderReportsBothSections(t *testing.T) {
	stats := NewStatsCollector(model.NewMetadata(), Options{View: "network"})
	findings := NewFindingsCollector(model.NewMetadata())
	for i, state := range []string{"PRIMARY", "SECONDARY"} {
		row := statsTestRow(int64(i+1), map[string]any{"activeConn": 1.0, "rsState": state})
		stats.Add(row)
		findings.Add(row)
	}
	statsReport, findingsReport := stats.Report(), findings.Report()
	opts := Options{View: "network", Report: "stats, findings", MetricsRange: stats.MetricsRange(), SampleCount: stats.Rows(), TimeLocation: time.UTC}

	var text bytes.Buffer
	if err := RenderReportsText(&text, model.NewMetadata(), &statsReport, &findingsReport, opts); err != nil {
		t.Fatal(err)
	}
	out := text.String()
	statsAt, findingsAt := strings.Index(out, "Stats: 2 rows"), strings.Index(out, "Findings: 1 events")
	if statsAt < 0 || findingsAt < statsAt || !strings.Contains(out, "PRIMARY -> SECONDARY") || !strings.Contains(out, "| stats, findings") {
		t.Fatalf("text:\n%s", out)
	}

	var js bytes.Buffer
	if err := RenderReportsJSON(&js, model.NewMetadata(), nil, &statsReport, &findingsReport, opts); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Reports  []string `json:"reports"`
		Findings struct {
			Events []map[string]any `json:"events"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Join(payload.Reports, ",") != "stats,findings" || len(payload.Findings.Events) != 1 {
		t.Fatalf("payload=%+v", payload)
	}
	e := payload.Findings.Events[0]
	if e["type"] != "rsState" || e["from"] != "PRIMARY" || e["to"] != "SECONDARY" || e["time"] != "1970-01-01T00:00:02Z" {
		t.Fatalf("event=%v", e)
	}
	if _, ok := e["pid"]; ok {
		t.Fatalf("unrelated fields should be omitted: %v", e)
	}
}

func TestRenderFindingsEmpty(t *testing.T) {
	report := NewFindingsCollector(model.NewMetadata()).Report()
	var text bytes.Buffer
	renderFindingsSection(&text, report, time.UTC)
	if !strings.Contains(text.String(), "Findings: 0 events") || !strings.Contains(text.String(), "no restarts, gaps, or replica-set state changes") {
		t.Fatalf("text=%q", text.String())
	}
	var js bytes.Buffer
	if err := json.NewEncoder(&js).Encode(report); err != nil || !strings.Contains(js.String(), `"events":[]`) {
		t.Fatalf("json=%s err=%v", js.String(), err)
	}
}

func TestFormatGapDuration(t *testing.T) {
	for seconds, want := range map[float64]string{
		61:      "1m1s",
		379:     "6m19s",
		3600:    "1h0m0s",
		86400:   "1d",
		1826664: "21d3h24m24s",
	} {
		if got := formatGapDuration(seconds); got != want {
			t.Fatalf("formatGapDuration(%v)=%q want %q", seconds, got, want)
		}
	}
}
