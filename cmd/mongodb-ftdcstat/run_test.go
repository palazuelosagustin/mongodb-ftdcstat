package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/discovery"
	"mongodb-ftdcstat/internal/ftdc"
	"mongodb-ftdcstat/internal/model"
	"mongodb-ftdcstat/internal/render"
	"mongodb-ftdcstat/internal/webui"
)

func TestTableOutputStreamingMatchesBatchRender(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}

	for _, view := range []string{"summary", "server", "wt", "system", "network", "repl"} {
		t.Run(view, func(t *testing.T) {
			rows, metadata, err := deriveRows(files, view, false, false)
			if err != nil {
				t.Fatal(err)
			}
			renderOpts := render.Options{View: view, TimeLocation: time.UTC, MetricsRange: render.MetricsRangeFromRows(rows)}

			var batch bytes.Buffer
			if err := render.Render(&batch, metadata, nil, rows, renderOpts); err != nil {
				t.Fatal(err)
			}

			var stream bytes.Buffer
			renderer, err := render.NewStreamingRenderer(&stream, metadata, renderOpts)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if err := renderer.RenderRow(row); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Close(); err != nil {
				t.Fatal(err)
			}

			if stream.String() != batch.String() {
				batchLines := bytes.Split(batch.Bytes(), []byte("\n"))
				streamLines := bytes.Split(stream.Bytes(), []byte("\n"))
				for i := 0; i < len(batchLines) && i < len(streamLines); i++ {
					if string(batchLines[i]) != string(streamLines[i]) {
						t.Fatalf("streaming table output mismatch for view %s at line %d\nbatch:  %q\nstream: %q", view, i+1, batchLines[i], streamLines[i])
					}
				}
				t.Fatalf("streaming table output mismatch for view %s: line counts batch=%d stream=%d", view, len(batchLines), len(streamLines))
			}
		})
	}
}

func TestStreamMetricsRangeMatchesDerivedRows(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}

	rows, metadata, err := deriveRows(files, "summary", false, false)
	if err != nil {
		t.Fatal(err)
	}
	input := captureInput{
		reader:     ftdc.NewNativeReader(),
		files:      files,
		readerOpts: ftdc.ReaderOptionsFor("summary", false, false),
		metadata:   metadata,
		streamerOpts: derive.Options{
			IntervalSeconds: 60,
			GapThreshold:    600 * time.Second,
			Metadata:        metadata,
			TimeLocation:    time.UTC,
		},
	}
	got, err := streamMetricsRange(input)
	if err != nil {
		t.Fatal(err)
	}
	want := render.MetricsRangeFromRows(rows)
	if !got.Start.Equal(want.Start) || !got.End.Equal(want.End) {
		t.Fatalf("metrics range mismatch: got=%s..%s want=%s..%s", got.Start.UTC().Format(time.RFC3339), got.End.UTC().Format(time.RFC3339), want.Start.UTC().Format(time.RFC3339), want.End.UTC().Format(time.RFC3339))
	}
}

func TestMongosViewsRenderRouterAndConnPoolSections(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "mongos.diagnostic.data")
	if _, err := os.Stat(root); err != nil {
		t.Skip("mongos.diagnostic.data sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}

	rows, metadata, err := deriveRows(files, "summary", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ProcessKind() != model.ProcessKindMongos {
		t.Fatalf("processKind=%q", metadata.ProcessKind())
	}

	var summary bytes.Buffer
	if err := render.Render(&summary, metadata, nil, rows, render.Options{View: "summary", TimeLocation: time.UTC}); err != nil {
		t.Fatal(err)
	}
	summaryOut := summary.String()
	for _, want := range []string{"|                     router", "|                                    connPool", "--- mongos process:"} {
		if !strings.Contains(summaryOut, want) {
			t.Fatalf("summary output missing %q:\n%s", want, summaryOut)
		}
	}
	for _, forbidden := range []string{"|     replication", "|                               wiredTiger"} {
		if strings.Contains(summaryOut, forbidden) {
			t.Fatalf("summary output should not contain %q:\n%s", forbidden, summaryOut)
		}
	}

	var repl bytes.Buffer
	if err := render.Render(&repl, metadata, nil, rows, render.Options{View: "repl", TimeLocation: time.UTC}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repl.String(), "|                     router") {
		t.Fatalf("repl output should render router section:\n%s", repl.String())
	}

	var wt bytes.Buffer
	if err := render.Render(&wt, metadata, nil, rows, render.Options{View: "wt", TimeLocation: time.UTC}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wt.String(), "|                                    connPool") {
		t.Fatalf("wt output should render connPool section for mongos:\n%s", wt.String())
	}
}

func TestMongosSummaryJSONUsesRouterAndConnPoolSections(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "mongos.diagnostic.data")
	if _, err := os.Stat(root); err != nil {
		t.Skip("mongos.diagnostic.data sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}
	rows, metadata, err := deriveRows(files, "summary", false, false)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := render.RenderJSON(&buf, metadata, nil, rows, render.Options{View: "summary", JSON: true, TimeLocation: time.UTC}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rowsJSON := payload["rows"].([]any)
	if len(rowsJSON) == 0 {
		t.Fatal("expected rows in summary JSON")
	}
	first := rowsJSON[0].(map[string]any)
	if _, ok := first["router"]; !ok {
		t.Fatalf("summary JSON missing router section: %#v", first)
	}
	if _, ok := first["connPool"]; !ok {
		t.Fatalf("summary JSON missing connPool section: %#v", first)
	}
	if _, ok := first["replication"]; ok {
		t.Fatalf("summary JSON should not contain replication section for mongos: %#v", first)
	}
	if _, ok := first["wiredTiger"]; ok {
		t.Fatalf("summary JSON should not contain wiredTiger section for mongos: %#v", first)
	}
}

func deriveRows(files []discovery.MetricFile, view string, verbose, pressure bool) ([]derive.Row, model.Metadata, error) {
	reader := ftdc.NewNativeReader()
	metadata, _, err := reader.ReadMetadataFiles(files)
	if err != nil {
		return nil, model.Metadata{}, err
	}
	readerOpts := ftdc.ReaderOptionsForKind(metadata.ProcessKind(), view, verbose, pressure)
	streamer := derive.NewStreamer(derive.Options{
		IntervalSeconds: 60,
		GapThreshold:    600 * time.Second,
		Metadata:        metadata,
		TimeLocation:    time.UTC,
	})
	var rows []derive.Row
	if _, err := reader.StreamFiles(files, readerOpts, func(sample model.MetricSample) error {
		if row, ok := streamer.Add(sample); ok {
			rows = append(rows, row)
		}
		return nil
	}); err != nil {
		return nil, model.Metadata{}, err
	}
	return rows, metadata, nil
}

type fakeWebServer struct {
	address   string
	listenArg string
	served    bool
	closed    bool
	serveErr  error
}

func (s *fakeWebServer) Listen(listen string) (string, error) {
	s.listenArg = listen
	return s.address, nil
}

func (s *fakeWebServer) Serve() error {
	s.served = true
	return s.serveErr
}

func (s *fakeWebServer) Close() error {
	s.closed = true
	return nil
}

func TestServeRenderedWebOutputPrintsEnabledLinksAndKeepAliveMessage(t *testing.T) {
	metadata := model.NewMetadata()
	rows := []derive.Row{{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"activeConn": 11.0,
		},
	}}
	renderOpts := render.Options{
		View:         "network",
		TimeLocation: time.UTC,
		MetricsRange: render.MetricsRangeFromRows(rows),
	}
	dataset := webui.BuildDataset(metadata, nil, rows, renderOpts, webui.Options{
		View:         "network",
		TimeLocation: time.UTC,
	})

	for _, tc := range []struct {
		name       string
		opts       cliOptions
		want       []string
		forbid     []string
		listenAddr string
	}{
		{
			name:       "web only",
			opts:       cliOptions{Web: true, Listen: "127.0.0.1:8080"},
			listenAddr: "http://127.0.0.1:8080",
			want: []string{
				"webUI\n  url: http://127.0.0.1:8080/\n",
				"HTTP server is running. Press Ctrl+C to stop.\n",
			},
			forbid: []string{"\nwebTUI\n"},
		},
		{
			name:       "tui only",
			opts:       cliOptions{TUI: true, Listen: "127.0.0.1:9090"},
			listenAddr: "http://127.0.0.1:9090",
			want: []string{
				"webTUI\n  url: http://127.0.0.1:9090/tui\n",
				"HTTP server is running. Press Ctrl+C to stop.\n",
			},
			forbid: []string{"\nwebUI\n"},
		},
		{
			name:       "web and tui",
			opts:       cliOptions{Web: true, TUI: true, Listen: ":7777"},
			listenAddr: "http://0.0.0.0:7777",
			want: []string{
				"webUI\n  url: http://127.0.0.1:7777/\n",
				"webTUI\n  url: http://127.0.0.1:7777/tui\n",
				"HTTP server is running. Press Ctrl+C to stop.\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeWebServer{address: tc.listenAddr}
			oldFactory := newWebServer
			newWebServer = func(dataset webui.Dataset) (webServer, error) {
				return fake, nil
			}
			defer func() { newWebServer = oldFactory }()

			var buf bytes.Buffer
			if err := serveRenderedWebOutput(&buf, metadata, nil, rows, renderOpts, tc.opts, dataset); err != nil {
				t.Fatal(err)
			}
			if !fake.served {
				t.Fatal("expected server.Serve to be called")
			}
			if fake.listenArg != tc.opts.Listen {
				t.Fatalf("listen arg=%q want=%q", fake.listenArg, tc.opts.Listen)
			}
			out := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in output:\n%s", want, out)
				}
			}
			for _, forbid := range tc.forbid {
				if strings.Contains(out, forbid) {
					t.Fatalf("unexpected %q in output:\n%s", forbid, out)
				}
			}
		})
	}
}

func TestServeRenderedWebOutputPropagatesServeError(t *testing.T) {
	fake := &fakeWebServer{address: "http://127.0.0.1:8080", serveErr: fmt.Errorf("serve failed")}
	oldFactory := newWebServer
	newWebServer = func(dataset webui.Dataset) (webServer, error) {
		return fake, nil
	}
	defer func() { newWebServer = oldFactory }()

	metadata := model.NewMetadata()
	rows := []derive.Row{{
		Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		Values: map[string]any{
			"activeConn": 11.0,
		},
	}}
	renderOpts := render.Options{View: "network", TimeLocation: time.UTC, MetricsRange: render.MetricsRangeFromRows(rows)}
	dataset := webui.BuildDataset(metadata, nil, rows, renderOpts, webui.Options{View: "network", TimeLocation: time.UTC})

	var buf bytes.Buffer
	err := serveRenderedWebOutput(&buf, metadata, nil, rows, renderOpts, cliOptions{Web: true}, dataset)
	if err == nil || err.Error() != "serve failed" {
		t.Fatalf("err=%v", err)
	}
	if !fake.served {
		t.Fatal("expected server.Serve to be called")
	}
}
