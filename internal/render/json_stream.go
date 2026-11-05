package render

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
)

// JSON output is written as a stream: the top-level keys before "rows",
// then one row at a time, then the keys after "rows". The bytes are the same
// as encoding the whole document with json.Encoder and two-space indent,
// whose map keys are sorted: avg, columns, metadata, rows, rsInfo, view,
// warnings. Memory no longer grows with the number of rows.

// JSONLayoutNeedsRows reports whether the --json row layout depends on the
// rows themselves: replica-set members first seen in rows (views with lag
// columns) and disk devices (--view io). Those need a first pass over the
// capture to collect the labels; other layouts can stream in one pass.
func JSONLayoutNeedsRows(metadata model.Metadata, opts Options) bool {
	if len(opts.Columns) > 0 {
		return false
	}
	if opts.View == "io" {
		return true
	}
	layout := layoutForView(opts.View, nil, nil, opts.Verbose, opts.Pressure, metadata.ProcessKind())
	for _, column := range layout.Columns {
		if column == "lagSLabel" {
			return true
		}
	}
	return false
}

// LabelCollector gathers the row-derived layout labels (replica-set lag
// columns and disk devices) during a first pass.
type LabelCollector struct {
	nodes   map[string]bool
	devices map[string]bool
}

func NewLabelCollector() *LabelCollector {
	return &LabelCollector{nodes: map[string]bool{}, devices: map[string]bool{}}
}

func (c *LabelCollector) Add(row derive.Row) {
	for key := range row.Values {
		if isNodeLagColumn(key) {
			c.nodes[key] = true
		}
	}
	if devices, ok := row.Values["disks"].([]map[string]any); ok {
		for _, device := range devices {
			if name, _ := device["disk"].(string); name != "" {
				c.devices[name] = true
			}
		}
	}
}

// rows returns one synthetic row holding every collected label, so the
// layout helpers that take rows produce the same labels as for all rows.
func (c *LabelCollector) rows() []derive.Row {
	if c == nil {
		return nil
	}
	values := map[string]any{}
	for node := range c.nodes {
		values[node] = nil
	}
	if len(c.devices) > 0 {
		names := make([]string, 0, len(c.devices))
		for name := range c.devices {
			names = append(names, name)
		}
		sort.Strings(names)
		disks := make([]map[string]any, 0, len(names))
		for _, name := range names {
			disks = append(disks, map[string]any{"disk": name})
		}
		values["disks"] = disks
	}
	return []derive.Row{{Values: values}}
}

// JSONStream writes the --json document row by row.
type JSONStream struct {
	w        io.Writer
	layout   tableLayout
	rsInfo   derive.ReplSetInfo
	opts     Options
	warnings []model.Warning
	rows     int
	err      error
}

// StartJSON writes the document up to the opening of "rows". labels may be
// nil when JSONLayoutNeedsRows is false.
func StartJSON(w io.Writer, metadata model.Metadata, warnings []model.Warning, opts Options, labels *LabelCollector) (*JSONStream, error) {
	return startJSON(w, metadata, warnings, opts, labels.rows())
}

func startJSON(w io.Writer, metadata model.Metadata, warnings []model.Warning, opts Options, labelRows []derive.Row) (*JSONStream, error) {
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	s := &JSONStream{
		w:        w,
		layout:   layoutFor(opts, replicationNodeLabels(rsInfo, labelRows), ioDeviceLabels(labelRows), metadata.ProcessKind()),
		rsInfo:   rsInfo,
		opts:     opts,
		warnings: warnings,
	}
	s.write("{\n")
	s.key("avg", avgPayload(opts.AvgBucket), true)
	if len(opts.Columns) > 0 {
		s.key("columns", opts.Columns, true)
	}
	s.key("metadata", MetadataSummary(metadata), true)
	s.write(`  "rows": [`)
	return s, s.err
}

// Row writes one row of the "rows" array.
func (s *JSONStream) Row(row derive.Row) error {
	if s.err != nil {
		return s.err
	}
	data, err := json.MarshalIndent(rowForJSON(row, s.layout), "    ", "  ")
	if err != nil {
		s.err = err
		return err
	}
	if s.rows == 0 {
		s.write("\n    ")
	} else {
		s.write(",\n    ")
	}
	s.write(string(data))
	s.rows++
	return s.err
}

// Close writes the end of "rows" and the keys after it.
func (s *JSONStream) Close() error {
	if s.rows == 0 {
		s.write("],\n")
	} else {
		s.write("\n  ],\n")
	}
	s.key("rsInfo", rsInfoForJSON(s.rsInfo), true)
	s.key("view", s.opts.View, true)
	s.key("warnings", s.warnings, false)
	s.write("}\n")
	return s.err
}

func (s *JSONStream) key(name string, value any, more bool) {
	if s.err != nil {
		return
	}
	data, err := json.MarshalIndent(value, "  ", "  ")
	if err != nil {
		s.err = err
		return
	}
	sep := "\n"
	if more {
		sep = ",\n"
	}
	s.write(fmt.Sprintf("  %q: %s%s", name, data, sep))
}

func (s *JSONStream) write(text string) {
	if s.err != nil {
		return
	}
	_, s.err = io.WriteString(s.w, text)
}
