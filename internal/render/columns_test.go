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

func TestEveryColumnIsDocumented(t *testing.T) {
	registered := map[string]bool{}
	for _, def := range metricRegistry {
		registered[def.Column] = true
		doc, ok := columnDocs[def.Column]
		if !ok {
			t.Errorf("column %q has no entry in columnDocs", def.Column)
			continue
		}
		if doc.Unit == "" || doc.Meaning == "" || doc.Source == "" {
			t.Errorf("column %q has an incomplete doc: %+v", def.Column, doc)
		}
	}
	for column := range columnDocs {
		if !registered[column] {
			t.Errorf("columnDocs has %q, which is not a registered column", column)
		}
	}
}

func TestResolveColumns(t *testing.T) {
	got, err := ResolveColumns([]string{" wtCache% ", "node2", "wtCache%", "", "appEvictMS/s"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "wtCache% node2 appEvictMS/s" {
		t.Fatalf("got %v", got)
	}
	for name, want := range map[string]string{
		"wtcache%":     `did you mean "wtCache%"`,
		"appEvictMs/s": `did you mean "appEvictMS/s"`,
		"bogus":        `unknown column "bogus" (see --list-columns)`,
	} {
		if _, err := ResolveColumns([]string{name}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err=%v want %q", name, err, want)
		}
	}
	if _, err := ResolveColumns([]string{"", " "}); err == nil {
		t.Fatal("empty list should fail")
	}
}

func TestLayoutForColumnsGroupsBySection(t *testing.T) {
	layout := layoutForColumns([]string{"wLatS", "wtCache%", "psiIoSome%", "node2", "rsState", "w_awaitS", "appEvictMS/s"})
	if got := strings.Join(layout.Columns, " "); got != "time lagSLabel node2 rsState wLatS psiIoSome% w_awaitS wtCache% appEvictMS/s" {
		t.Fatalf("columns=%s", got)
	}
	var names []string
	for _, s := range layout.Sections {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, " "); got != "replication server pressure io wiredTiger" {
		t.Fatalf("sections=%s", got)
	}
	if got := strings.Join(layoutForColumns([]string{"rsState"}).Columns, " "); got != "time rsState" {
		t.Fatalf("lagS label should only precede node columns: %s", got)
	}
}

func TestReadPlanForColumns(t *testing.T) {
	plan := ReadPlanForColumns([]string{"wtCache%", "psiIoSome%", "node1", "pingMS", "poolInUse"})
	if strings.Join(plan.Views, " ") != "repl system wt" || !plan.Pressure {
		t.Fatalf("plan=%+v", plan)
	}
	if plan := ReadPlanForColumns([]string{"w_awaitS"}); strings.Join(plan.Views, " ") != "system" || plan.Pressure {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestRenderColumnsTableFitsSectionLabels(t *testing.T) {
	row := derive.Row{Time: time.Unix(10, 0).UTC(), Values: map[string]any{"wLatS": 0.5, "wtCache%": 80.0}}
	var buf bytes.Buffer
	opts := Options{View: ColumnsView, Columns: []string{"wtCache%", "wLatS"}, TimeLocation: time.UTC}
	if err := Render(&buf, model.NewMetadata(), nil, []derive.Row{row}, opts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "| Columns | wtCache% wLatS ") {
		t.Fatalf("header missing Columns row:\n%s", out)
	}
	labels, header := firstTableHeader(out)
	if !strings.Contains(labels, "server") || !strings.Contains(labels, "wiredTiger") {
		t.Fatalf("section labels cut:\n%s", labels)
	}
	if got := normalizedTableLine(header); got != "datetime wLatS wtCache%" {
		t.Fatalf("header=%q", got)
	}
}

func TestListColumns(t *testing.T) {
	all := ListColumns("")
	byName := map[string]ColumnInfo{}
	for _, c := range all {
		byName[c.Column] = c
	}
	if len(all) != len(metricRegistry)+1 {
		t.Fatalf("listed %d columns, want every registered column plus node1..nodeN (%d)", len(all), len(metricRegistry)+1)
	}
	check := func(name, needs, process, view string) {
		t.Helper()
		c, ok := byName[name]
		if !ok {
			t.Fatalf("%s not listed", name)
		}
		if c.Needs != needs || strings.Join(c.Process, ",") != process || !hasView(c.Views, view) {
			t.Fatalf("%s=%+v", name, c)
		}
	}
	check("wtCache%", "", "mongod", "summary")
	check("appEvictMS/s", "--verbose", "mongod", "wt")
	check("psiIoSome%", "--pressure", "mongod,mongos", "system")
	check("pingMS", "", "mongos", "repl")
	check("node1..nodeN", "", "mongod", "repl")
	check("r/s", "", "mongod,mongos", "io")

	for _, c := range ListColumns("network") {
		if c.Section != "network" {
			t.Fatalf("--view network listed %+v", c)
		}
	}

	var js bytes.Buffer
	if err := RenderColumnList(&js, ListColumns("server"), true); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Columns []ColumnInfo `json:"columns"`
	}
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil || len(payload.Columns) == 0 || payload.Columns[0].Source == "" {
		t.Fatalf("json=%s err=%v", js.String(), err)
	}
}
