package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/discovery"
	"ftdcstat/internal/ftdc"
	"ftdcstat/internal/model"
	"ftdcstat/internal/render"
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
			renderOpts := render.Options{View: view, TimeLocation: time.UTC}

			var batch bytes.Buffer
			if err := render.Render(&batch, metadata, nil, rows, renderOpts); err != nil {
				t.Fatal(err)
			}

			var stream bytes.Buffer
			renderer, err := render.NewStreamingRenderer(&stream, metadata, nil, renderOpts)
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

func deriveRows(files []discovery.MetricFile, view string, verbose, pressure bool) ([]derive.Row, model.Metadata, error) {
	reader := ftdc.NewNativeReader()
	readerOpts := ftdc.ReaderOptionsFor(view, verbose, pressure)
	metadata, _, err := reader.ReadMetadataFiles(files)
	if err != nil {
		return nil, model.Metadata{}, err
	}
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
