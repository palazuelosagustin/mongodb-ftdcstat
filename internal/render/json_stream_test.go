package render

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
	"mongodb-ftdcstat/internal/version"
)

// encodeWholeDocument is the pre-streaming --json implementation: build the
// whole document and encode it at once. The stream must match it byte for
// byte.
func encodeWholeDocument(t *testing.T, metadata model.Metadata, warnings []model.Warning, rows []derive.Row, opts Options) string {
	t.Helper()
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	layout := layoutFor(opts, replicationNodeLabels(rsInfo, rows), ioDeviceLabels(rows), metadata.ProcessKind())
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowForJSON(row, layout))
	}
	payload := map[string]any{
		"metadata":    MetadataSummary(metadata),
		"rsInfo":      rsInfoForJSON(rsInfo),
		"warnings":    warnings,
		"view":        opts.View,
		"toolVersion": version.String(),
		"avg":         avgPayload(opts.AvgBucket),
		"rows":        items,
	}
	if len(opts.Columns) > 0 {
		payload["columns"] = opts.Columns
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func jsonStreamTestRows() []derive.Row {
	base := time.Date(2026, 6, 4, 23, 0, 0, 0, time.UTC)
	first := derive.Row{
		Time:          base,
		ProcessMarker: "--- mongod process: pid=1 start=2026-06-04T22:00:00Z ---",
		Values: map[string]any{
			"rsState": "PRIMARY", "node1": 0.0, "majLagS": 0.0, "wtCache%": 51.5, "qry/s": 3.0,
			"disks": []map[string]any{{"disk": "sdb", "util%": 10.0}, {"disk": "sda", "util%": 5.0}},
		},
	}
	second := derive.Row{
		Time:   base.Add(time.Second),
		Marker: "gap <97s> & reset", // HTML-sensitive characters are escaped by encoding/json
		Values: map[string]any{"rsState": "SECONDARY", "node1": 1.5, "node4": 2.0, "wtCache%": nil},
	}
	return []derive.Row{first, second}
}

func TestJSONStreamMatchesWholeDocumentEncoding(t *testing.T) {
	metadata := testMetadata()
	cases := []struct {
		name     string
		opts     Options
		rows     []derive.Row
		warnings []model.Warning
	}{
		{"summary", Options{View: "summary"}, jsonStreamTestRows(), nil},
		{"repl verbose", Options{View: "repl", Verbose: true}, jsonStreamTestRows(), []model.Warning{{Source: "metrics.1", Message: "x"}}},
		{"io", Options{View: "io"}, jsonStreamTestRows(), []model.Warning{}},
		{"wt avg", Options{View: "wt", AvgBucket: 5 * time.Minute}, jsonStreamTestRows(), nil},
		{"columns", Options{View: ColumnsView, Columns: []string{"wtCache%", "node1", "rsState"}}, jsonStreamTestRows(), nil},
		{"no rows", Options{View: "wt"}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := encodeWholeDocument(t, metadata, tc.warnings, tc.rows, tc.opts)

			var got bytes.Buffer
			if err := RenderJSON(&got, metadata, tc.warnings, tc.rows, tc.opts); err != nil {
				t.Fatal(err)
			}
			if got.String() != want {
				t.Fatalf("RenderJSON differs from whole-document encoding:\n--- got\n%s\n--- want\n%s", got.String(), want)
			}

			// The CLI path: labels from a first pass, then rows streamed.
			var labels *LabelCollector
			if JSONLayoutNeedsRows(metadata, tc.opts) {
				labels = NewLabelCollector()
				for _, row := range tc.rows {
					labels.Add(row)
				}
			}
			var streamed bytes.Buffer
			stream, err := StartJSON(&streamed, metadata, tc.warnings, tc.opts, labels)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range tc.rows {
				if err := stream.Row(row); err != nil {
					t.Fatal(err)
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			if streamed.String() != want {
				t.Fatalf("streamed JSON differs:\n--- got\n%s\n--- want\n%s", streamed.String(), want)
			}
		})
	}
}

func TestJSONLayoutNeedsRows(t *testing.T) {
	mongod := testMetadataWithProcessKind(model.ProcessKindMongod)
	mongos := testMetadataWithProcessKind(model.ProcessKindMongos)
	for _, tc := range []struct {
		metadata model.Metadata
		opts     Options
		want     bool
	}{
		{mongod, Options{View: "summary"}, true},
		{mongod, Options{View: "repl"}, true},
		{mongod, Options{View: "io"}, true},
		{mongod, Options{View: "wt"}, false},
		{mongod, Options{View: "system", Verbose: true}, false},
		{mongod, Options{View: ColumnsView, Columns: []string{"node1"}}, false},
		{mongos, Options{View: "summary"}, false},
		{mongos, Options{View: "io"}, true},
	} {
		if got := JSONLayoutNeedsRows(tc.metadata, tc.opts); got != tc.want {
			t.Fatalf("%s %+v: got %v want %v", tc.metadata.ProcessKind(), tc.opts, got, tc.want)
		}
	}
}

func testMetadataWithProcessKind(kind string) model.Metadata {
	m := model.NewMetadata()
	m.AddDocument(time.Unix(0, 0), "metadata", map[string]any{"serverStatus": map[string]any{"process": kind}})
	return m
}
