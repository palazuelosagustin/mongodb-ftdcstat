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

func statsTestRow(sec int64, values map[string]any) derive.Row {
	return derive.Row{Time: time.Unix(sec, 0).UTC(), Values: values}
}

func findStatsColumn(t *testing.T, report StatsReport, column string) StatsColumn {
	t.Helper()
	for _, col := range report.Columns {
		if col.Column == column {
			return col
		}
	}
	t.Fatalf("column %s not in report", column)
	return StatsColumn{}
}

func TestStatsCollectorServerView(t *testing.T) {
	c := NewStatsCollector(model.NewMetadata(), Options{View: "server"})
	c.Add(statsTestRow(1, map[string]any{"qTot": 0.0, "ins/s": 10.0, "wLatS": nil}))
	c.Add(statsTestRow(2, map[string]any{"qTot": 4.0, "ins/s": 30.0, "wLatS": 0.5}))
	// Restart row: rates are omitted entirely.
	restart := statsTestRow(3, map[string]any{"qTot": 2.0})
	restart.ProcessMarker = "--- mongod restart detected: pid=2 ---"
	c.Add(restart)
	gap := statsTestRow(100, map[string]any{"qTot": 1.0, "ins/s": 20.0})
	gap.Marker = "gap 97s: rate baseline reset"
	c.Add(gap)

	report := c.Report()
	if report.Rows != 4 || report.Restarts != 1 || report.Gaps != 1 {
		t.Fatalf("rows=%d restarts=%d gaps=%d", report.Rows, report.Restarts, report.Gaps)
	}
	var names []string
	for _, col := range report.Columns {
		names = append(names, col.Column)
	}
	if got, want := strings.Join(names, " "), strings.Join(columnsForSection("server"), " "); got != want {
		t.Fatalf("columns=%q want %q", got, want)
	}

	ins := findStatsColumn(t, report, "ins/s")
	if ins.N != 3 || ins.NullPct != 25 || *ins.Min != 10 || *ins.Max != 30 || *ins.Avg != 20 {
		t.Fatalf("ins/s=%+v", ins)
	}
	if ins.MaxAt == nil || *ins.MaxAt != "1970-01-01T00:00:02Z" {
		t.Fatalf("ins/s maxAt=%v", ins.MaxAt)
	}
	wLat := findStatsColumn(t, report, "wLatS")
	if wLat.N != 1 || wLat.NullPct != 75 {
		t.Fatalf("wLatS=%+v", wLat)
	}
	never := findStatsColumn(t, report, "cmd/s")
	if never.N != 0 || never.NullPct != 100 || never.Max != nil || never.MaxAt != nil {
		t.Fatalf("cmd/s=%+v", never)
	}
}

func TestStatsCollectorColumnAppearingLateCountsEarlierRowsAsNull(t *testing.T) {
	c := NewStatsCollector(model.NewMetadata(), Options{View: "server"})
	c.Add(statsTestRow(1, map[string]any{}))
	c.Add(statsTestRow(2, map[string]any{}))
	c.Add(statsTestRow(3, map[string]any{"qry/s": 6.0}))
	qry := findStatsColumn(t, c.Report(), "qry/s")
	if qry.N != 1 || qry.NullPct < 66.6 || qry.NullPct > 66.7 {
		t.Fatalf("qry/s=%+v", qry)
	}
}

func TestStatsCollectorReplicationStatesAndNodes(t *testing.T) {
	c := NewStatsCollector(model.NewMetadata(), Options{View: "repl"})
	c.Add(statsTestRow(1, map[string]any{"rsState": "PRIMARY", "node1": 0.0, "majLagS": 0.0}))
	c.Add(statsTestRow(2, map[string]any{"rsState": "SECONDARY", "node1": 1.0, "node2": 3.0}))
	c.Add(statsTestRow(3, map[string]any{"rsState": "SECONDARY", "node1": 2.0, "node2": 5.0}))

	report := c.Report()
	node2 := findStatsColumn(t, report, "node2")
	if node2.Section != "replication" || node2.N != 2 || *node2.Max != 5 {
		t.Fatalf("node2=%+v", node2)
	}
	if len(report.States) != 1 {
		t.Fatalf("states=%+v", report.States)
	}
	states := report.States[0]
	if states.Column != "rsState" || states.Changes != 1 || len(states.Values) != 2 || states.Values[0].Value != "PRIMARY" || states.Values[1].Rows != 2 {
		t.Fatalf("rsState=%+v", states)
	}
}

func TestStatsCollectorIOViewPerDevice(t *testing.T) {
	c := NewStatsCollector(model.NewMetadata(), Options{View: "io"})
	for i, util := range []float64{10, 90} {
		c.Add(statsTestRow(int64(i+1), map[string]any{
			"util%": 99.0,
			"disks": []map[string]any{
				{"disk": "sdb", "util%": util},
				{"disk": "sda", "util%": util / 2},
			},
		}))
	}
	report := c.Report()
	var got []string
	for _, col := range report.Columns {
		if col.Column == "util%" {
			got = append(got, col.Subsection)
			if col.Subsection == "sdb" && *col.Max != 90 {
				t.Fatalf("sdb util%%=%+v", col)
			}
		}
	}
	if strings.Join(got, ",") != "sda,sdb" {
		t.Fatalf("util%% devices=%v", got)
	}
}

func TestRenderStatsTextAndJSON(t *testing.T) {
	c := NewStatsCollector(model.NewMetadata(), Options{View: "network"})
	c.Add(statsTestRow(1, map[string]any{"activeConn": 8.0, "idleConn": 2.0}))
	c.Add(statsTestRow(2, map[string]any{"activeConn": 9.0, "idleConn": 2.0}))
	opts := Options{View: "network", Report: "stats", MetricsRange: c.MetricsRange(), SampleCount: c.Rows(), TimeLocation: time.UTC}
	report := c.Report()

	var text bytes.Buffer
	if err := RenderStatsText(&text, model.NewMetadata(), report, opts); err != nil {
		t.Fatal(err)
	}
	out := text.String()
	for _, want := range []string{"| Mode", "report stats", "Stats: 2 rows, 0 restarts, 0 gaps", "section  column", "network  activeConn"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text missing %q:\n%s", want, out)
		}
	}
	// Integer columns keep a decimal in avg only.
	if !strings.Contains(out, "8.5") {
		t.Fatalf("activeConn avg not 8.5:\n%s", out)
	}

	var js bytes.Buffer
	if err := RenderStatsJSON(&js, model.NewMetadata(), nil, report, opts); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Report string `json:"report"`
		Range  struct {
			Start string `json:"start"`
		} `json:"range"`
		Stats struct {
			Rows    int `json:"rows"`
			Columns []struct {
				Column string   `json:"column"`
				Max    *float64 `json:"max"`
				MaxAt  *string  `json:"maxAt"`
			} `json:"columns"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Report != "stats" || payload.Stats.Rows != 2 || payload.Range.Start != "1970-01-01T00:00:01Z" {
		t.Fatalf("payload=%+v", payload)
	}
	last := payload.Stats.Columns[len(payload.Stats.Columns)-1]
	if last.Column != "totalCreated/s" || last.Max != nil || last.MaxAt != nil {
		t.Fatalf("missing column should be null: %+v", last)
	}
}
