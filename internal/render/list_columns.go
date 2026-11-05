package render

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"mongodb-ftdcstat/internal/model"
)

// ColumnInfo describes one column for --list-columns.
type ColumnInfo struct {
	Column  string   `json:"column"`
	Section string   `json:"section"`
	Unit    string   `json:"unit"`
	Meaning string   `json:"meaning"`
	Source  string   `json:"source"`
	Needs   string   `json:"needs"`   // "", "--verbose", or "--pressure"
	Process []string `json:"process"` // mongod and/or mongos
	Views   []string `json:"views"`   // e.g. "summary", "wt --verbose"
}

var listViews = []string{"summary", "server", "repl", "network", "system", "io", "wt"}

// ListColumns returns every column with where it appears. The views and
// process kinds are computed from the same layouts the tables use. With a
// non-empty view, only columns shown by that view (with any flags) are kept.
func ListColumns(view string) []ColumnInfo {
	type where struct {
		views   map[string]bool
		process map[string]bool
		plain   bool
		verbose bool
	}
	found := map[string]*where{}
	var order []string
	add := func(column, label, process string, plain, verbose bool) {
		if column == "time" || column == "lagSLabel" {
			return
		}
		if _, metric, ok := parseIOColumnKey(column); ok {
			column = metric
		}
		if isNodeLagColumn(column) {
			column = "node1..nodeN"
		}
		w := found[column]
		if w == nil {
			w = &where{views: map[string]bool{}, process: map[string]bool{}}
			found[column] = w
			order = append(order, column)
		}
		w.views[label] = true
		w.process[process] = true
		w.plain = w.plain || plain
		w.verbose = w.verbose || verbose
	}
	columnSet := func(l tableLayout) map[string]bool {
		set := map[string]bool{}
		for _, c := range l.Columns {
			set[c] = true
		}
		return set
	}
	for _, process := range []string{model.ProcessKindMongod, model.ProcessKindMongos} {
		for _, v := range listViews {
			nodes, devices := []string{"node1"}, []string{"sda"}
			base := layoutForView(v, nodes, devices, false, false, process)
			baseSet := columnSet(base)
			for _, c := range base.Columns {
				add(c, v, process, true, false)
			}
			if verboseAllowed(v) {
				for _, c := range layoutForView(v, nodes, devices, true, false, process).Columns {
					if !baseSet[c] {
						add(c, v+" --verbose", process, false, true)
					}
				}
			}
			if v == "system" {
				for _, c := range layoutForView(v, nodes, devices, false, true, process).Columns {
					if !baseSet[c] {
						add(c, v+" --pressure", process, false, false)
					}
				}
			}
		}
	}

	var out []ColumnInfo
	for _, column := range order {
		w := found[column]
		info := ColumnInfo{Column: column, Section: columnLayoutSection(column)}
		doc := nodeLagDoc
		if column == "node1..nodeN" {
			info.Section = "replication"
		} else {
			doc = columnDocs[column]
		}
		info.Unit, info.Meaning, info.Source = doc.Unit, doc.Meaning, doc.Source
		switch {
		case w.plain:
		case w.verbose:
			info.Needs = "--verbose"
		default:
			info.Needs = "--pressure"
		}
		for _, p := range []string{model.ProcessKindMongod, model.ProcessKindMongos} {
			if w.process[p] {
				info.Process = append(info.Process, p)
			}
		}
		for label := range w.views {
			info.Views = append(info.Views, label)
		}
		sort.Strings(info.Views)
		if view != "" && !hasView(info.Views, view) {
			continue
		}
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return sectionRank(out[i].Section) < sectionRank(out[j].Section)
	})
	return out
}

func verboseAllowed(view string) bool {
	switch view {
	case "server", "repl", "wt", "system", "network":
		return true
	}
	return false
}

func hasView(views []string, view string) bool {
	for _, v := range views {
		if v == view || strings.HasPrefix(v, view+" ") {
			return true
		}
	}
	return false
}

func sectionRank(section string) int {
	for i, name := range columnSectionOrder {
		if name == section {
			return i
		}
	}
	return len(columnSectionOrder)
}

// RenderColumnList writes --list-columns output: a table, or JSON.
func RenderColumnList(w io.Writer, columns []ColumnInfo, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"columns": columns})
	}
	lines := [][]string{{"section", "column", "unit", "needs", "process", "meaning"}}
	for _, c := range columns {
		needs := c.Needs
		if needs == "" {
			needs = "-"
		}
		process := strings.Join(c.Process, ",")
		if len(c.Process) == 2 {
			process = "both"
		}
		lines = append(lines, []string{c.Section, c.Column, c.Unit, needs, process, c.Meaning})
	}
	writeLeftTable(w, lines)
	fmt.Fprintln(w, "\nUse a column with --columns, or its view (plus --verbose/--pressure when listed).")
	fmt.Fprintln(w, "--list-columns --json adds each column's source and the views that show it.")
	return nil
}

func writeLeftTable(w io.Writer, lines [][]string) {
	writeAlignedTable(w, lines, len(lines[0]))
}
