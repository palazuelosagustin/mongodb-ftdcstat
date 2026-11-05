package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
	"mongodb-ftdcstat/internal/stats"
)

// StatsCollector accumulates per-column statistics over derived rows for
// --report stats. It keeps every candidate column while streaming and picks
// the view's columns at the end, because replica-set member and disk device
// columns are only known after the rows have been seen.
type StatsCollector struct {
	metadata model.Metadata
	opts     Options
	ioView   bool

	rows     uint64
	start    time.Time
	end      time.Time
	restarts uint64
	gaps     uint64

	numeric map[string]*stats.Column
	text    map[string]*stats.States
	nodes   map[string]bool
	devices map[string]bool
	seen    map[string]bool
}

func NewStatsCollector(metadata model.Metadata, opts Options) *StatsCollector {
	return &StatsCollector{
		metadata: metadata,
		opts:     opts,
		ioView:   opts.View == "io",
		numeric:  map[string]*stats.Column{},
		text:     map[string]*stats.States{},
		nodes:    map[string]bool{},
		devices:  map[string]bool{},
		seen:     map[string]bool{},
	}
}

func (c *StatsCollector) Add(row derive.Row) {
	if row.Time.IsZero() {
		return
	}
	c.rows++
	if c.start.IsZero() {
		c.start = row.Time
	}
	c.end = row.Time
	if strings.Contains(row.ProcessMarker, "restart detected") {
		c.restarts++
	}
	if row.Marker != "" {
		c.gaps++
	}

	seen := c.seen
	clear(seen)
	for key, value := range row.Values {
		if key == "disks" {
			continue
		}
		if isNodeLagColumn(key) {
			c.nodes[key] = true
		}
		c.addValue(key, row.Time, value)
		seen[key] = true
	}
	if c.ioView {
		if devices, ok := row.Values["disks"].([]map[string]any); ok {
			for _, device := range devices {
				name, _ := device["disk"].(string)
				if name == "" {
					continue
				}
				c.devices[name] = true
				for metric, value := range device {
					if metric == "disk" {
						continue
					}
					key := ioColumnKey(name, metric)
					c.addValue(key, row.Time, value)
					seen[key] = true
				}
			}
		}
	}
	// Columns already known but absent from this row are nulls for it.
	for key, column := range c.numeric {
		if !seen[key] {
			column.Add(row.Time, 0, false)
		}
	}
	for key, states := range c.text {
		if !seen[key] {
			states.Add("")
		}
	}
}

func (c *StatsCollector) addValue(key string, t time.Time, value any) {
	if s, ok := value.(string); ok {
		states := c.text[key]
		if states == nil {
			states = c.newStates()
			c.text[key] = states
		}
		states.Add(s)
		return
	}
	column := c.numeric[key]
	if column == nil {
		column = c.newColumn()
		c.numeric[key] = column
	}
	v, ok := numericValue(value)
	column.Add(t, v, ok)
}

// newColumn and newStates backfill nulls for the rows seen before the column
// first appeared, so every column reports against the same row count.
func (c *StatsCollector) newColumn() *stats.Column {
	column := stats.NewColumn()
	for i := uint64(1); i < c.rows; i++ {
		column.Add(time.Time{}, 0, false)
	}
	return column
}

func (c *StatsCollector) newStates() *stats.States {
	states := stats.NewStates()
	for i := uint64(1); i < c.rows; i++ {
		states.Add("")
	}
	return states
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func (c *StatsCollector) MetricsRange() MetricsRange {
	return MetricsRange{Start: c.start, End: c.end}
}

func (c *StatsCollector) Rows() int {
	return int(c.rows)
}

// StatsColumn is one numeric column of the stats report.
type StatsColumn struct {
	Section    string     `json:"section"`
	Subsection string     `json:"subsection,omitempty"`
	Column     string     `json:"column"`
	N          uint64     `json:"n"`
	NullPct    float64    `json:"nullPct"`
	Min        *float64   `json:"min"`
	Avg        *float64   `json:"avg"`
	P50        *float64   `json:"p50"`
	P95        *float64   `json:"p95"`
	P99        *float64   `json:"p99"`
	Max        *float64   `json:"max"`
	MaxAt      *time.Time `json:"maxAt"`

	key string
}

// StatsStateCount is one value of a text column.
type StatsStateCount struct {
	Value string  `json:"value"`
	Rows  uint64  `json:"rows"`
	Pct   float64 `json:"pct"`
}

// StatsStates is one text column of the stats report, such as rsState.
type StatsStates struct {
	Section string            `json:"section"`
	Column  string            `json:"column"`
	Values  []StatsStateCount `json:"values"`
	Changes uint64            `json:"changes"`
}

// StatsReport is the --report stats result.
type StatsReport struct {
	Rows     uint64        `json:"rows"`
	Restarts uint64        `json:"restarts"`
	Gaps     uint64        `json:"gaps"`
	Columns  []StatsColumn `json:"columns"`
	States   []StatsStates `json:"states"`
}

func (c *StatsCollector) Report() StatsReport {
	report := StatsReport{Rows: c.rows, Restarts: c.restarts, Gaps: c.gaps, Columns: []StatsColumn{}, States: []StatsStates{}}
	rsInfo := derive.ReplSetInfoFromMetadata(c.metadata)
	layout := layoutForView(c.opts.View, c.nodeLabels(rsInfo), sortedSet(c.devices), c.opts.Verbose, c.opts.Pressure, c.metadata.ProcessKind())
	for _, section := range layout.Sections {
		for _, key := range layout.Columns[section.Start:section.End] {
			if key == "time" || key == "lagSLabel" {
				continue
			}
			display := key
			if _, metric, ok := parseIOColumnKey(key); ok {
				display = metric
			}
			if states, ok := c.text[key]; ok || isTextColumn(key) {
				report.States = append(report.States, statesSummary(section.Name, display, states))
				continue
			}
			report.Columns = append(report.Columns, c.columnSummary(section, display, key))
		}
	}
	return report
}

func (c *StatsCollector) nodeLabels(rsInfo derive.ReplSetInfo) []string {
	labels := replicationNodeLabels(rsInfo, nil)
	known := map[string]bool{}
	for _, label := range labels {
		known[label] = true
	}
	var extra []derive.Row
	for label := range c.nodes {
		if !known[label] {
			extra = append(extra, derive.Row{Values: map[string]any{label: nil}})
		}
	}
	if len(extra) == 0 {
		return labels
	}
	return replicationNodeLabels(rsInfo, extra)
}

func (c *StatsCollector) columnSummary(section tableSection, display, key string) StatsColumn {
	out := StatsColumn{Section: section.Name, Subsection: section.Subsection, Column: display, key: key}
	column, ok := c.numeric[key]
	if !ok {
		out.NullPct = 100
		return out
	}
	summary := column.Summary()
	out.N = summary.N
	if c.rows > 0 {
		out.NullPct = float64(summary.Nulls) / float64(c.rows) * 100
	}
	out.Min, out.Avg, out.P50, out.P95, out.P99, out.Max = summary.Min, summary.Avg, summary.P50, summary.P95, summary.P99, summary.Max
	if summary.N > 0 {
		maxAt := summary.MaxAt
		out.MaxAt = &maxAt
	}
	return out
}

func statesSummary(section, column string, states *stats.States) StatsStates {
	out := StatsStates{Section: section, Column: column, Values: []StatsStateCount{}}
	if states == nil {
		return out
	}
	out.Changes = states.Changes
	for _, value := range states.Values() {
		rows := states.Counts[value]
		out.Values = append(out.Values, StatsStateCount{Value: value, Rows: rows, Pct: float64(rows) / float64(states.Rows) * 100})
	}
	return out
}

// RenderStatsText writes the CLI header followed by the stats tables.
func RenderStatsText(w io.Writer, metadata model.Metadata, report StatsReport, opts Options) error {
	if err := RenderCLIHeader(w, metadata, opts); err != nil {
		return err
	}
	loc := opts.TimeLocation
	if loc == nil {
		loc = time.UTC
	}
	fmt.Fprintf(w, "Stats: %d rows, %d restarts, %d gaps; percentiles are within %.0f%%; max at is the first row with the max.\n\n",
		report.Rows, report.Restarts, report.Gaps, stats.RelativeAccuracy*100)

	lines := [][]string{{"section", "column", "n", "null%", "min", "avg", "p50", "p95", "p99", "max", "max at"}}
	for _, col := range report.Columns {
		section := col.Section
		if col.Subsection != "" {
			section += "/" + col.Subsection
		}
		maxAt := "-"
		if col.MaxAt != nil {
			maxAt = formatRowTime(*col.MaxAt, loc)
		}
		lines = append(lines, []string{
			section, col.Column, fmt.Sprint(col.N), fmt.Sprintf("%.1f", col.NullPct),
			statsValue(col.Min, col.key), statsAvg(col.Avg, col.key), statsValue(col.P50, col.key),
			statsValue(col.P95, col.key), statsValue(col.P99, col.key), statsValue(col.Max, col.key), maxAt,
		})
	}
	writeAlignedTable(w, lines, 2)

	for _, states := range report.States {
		parts := make([]string, 0, len(states.Values))
		for _, value := range states.Values {
			parts = append(parts, fmt.Sprintf("%s %.1f%%", value.Value, value.Pct))
		}
		values := strings.Join(parts, ", ")
		if values == "" {
			values = "-"
		}
		fmt.Fprintf(w, "\n%s %s: %s; %d changes\n", states.Section, states.Column, values, states.Changes)
	}
	return nil
}

// statsAvg keeps one decimal for columns that are otherwise printed as whole
// numbers, so an average of 8.6 connections is not shown as 9.
func statsAvg(v *float64, key string) string {
	if v != nil {
		switch metricFormat(key) {
		case "integer", "bool":
			return fmt.Sprintf("%.1f", *v)
		}
	}
	return statsValue(v, key)
}

// writeAlignedTable left-aligns the first leftCols columns and right-aligns
// the rest.
func writeAlignedTable(w io.Writer, lines [][]string, leftCols int) {
	var widths []int
	for _, line := range lines {
		for i, cell := range line {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], len(cell))
		}
	}
	for _, line := range lines {
		cells := make([]string, len(line))
		for i, cell := range line {
			if i < leftCols {
				cells[i] = fmt.Sprintf("%-*s", widths[i], cell)
			} else {
				cells[i] = fmt.Sprintf("%*s", widths[i], cell)
			}
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(cells, "  "), " "))
	}
}

func statsValue(v *float64, key string) string {
	if v == nil {
		return "-"
	}
	return format(*v, key)
}

// RenderStatsJSON writes the stats report as one JSON document.
func RenderStatsJSON(w io.Writer, metadata model.Metadata, warnings []model.Warning, report StatsReport, opts Options) error {
	if warnings == nil {
		warnings = []model.Warning{}
	}
	payload := map[string]any{
		"report":   "stats",
		"view":     opts.View,
		"metadata": metadata.Summary(),
		"rsInfo":   rsInfoForJSON(derive.ReplSetInfoFromMetadata(metadata)),
		"warnings": warnings,
		"range": map[string]any{
			"start": jsonTime(opts.MetricsRange.Start),
			"end":   jsonTime(opts.MetricsRange.End),
		},
		"percentileRelativeAccuracy": stats.RelativeAccuracy,
		"stats":                      report,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func jsonTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
