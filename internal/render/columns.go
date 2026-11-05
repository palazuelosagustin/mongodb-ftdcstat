package render

import (
	"fmt"
	"sort"
	"strings"
)

// ColumnsView is the Options.View value used with --columns.
const ColumnsView = "columns"

// columnSectionOrder is the order of sections in a --columns table. It
// follows the summary layouts: replication/router first, storage last.
var columnSectionOrder = []string{"replication", "router", "server", "network", "system", "pressure", "io", "wiredTiger", "connPool"}

// layoutFor returns the table layout for opts: the --columns selection when
// given, otherwise the selected view.
func layoutFor(opts Options, nodeLabels, deviceLabels []string, processKind string) tableLayout {
	if len(opts.Columns) > 0 {
		return layoutForColumns(opts.Columns)
	}
	return layoutForView(opts.View, nodeLabels, deviceLabels, opts.Verbose, opts.Pressure, processKind)
}

// layoutForColumns groups the requested columns into their sections, in
// columnSectionOrder, keeping the requested order inside each section.
func layoutForColumns(columns []string) tableLayout {
	bySection := map[string][]string{}
	for _, column := range columns {
		section := columnLayoutSection(column)
		bySection[section] = append(bySection[section], column)
	}
	var replication []string
	if cols := bySection["replication"]; len(cols) > 0 {
		for _, col := range cols {
			if isNodeLagColumn(col) {
				// The lag columns are introduced by the "lagS" label column.
				replication = append([]string{"lagSLabel"}, cols...)
				break
			}
		}
		if replication == nil {
			replication = cols
		}
	}
	var sections []namedColumns
	for _, name := range columnSectionOrder {
		if name == "replication" {
			continue
		}
		if cols := bySection[name]; len(cols) > 0 {
			sections = append(sections, namedColumns{Name: name, Columns: cols})
		}
	}
	return buildLayout(replication, sections)
}

// columnLayoutSection is the table section a column is shown in. PSI columns
// use the separate "pressure" section, as in --view system --pressure.
func columnLayoutSection(column string) string {
	if isNodeLagColumn(column) {
		return "replication"
	}
	def, ok := metricDefinitionForColumn(column)
	if !ok {
		return "other"
	}
	if def.PressureOnly {
		return "pressure"
	}
	return def.Section
}

// ResolveColumns validates --columns names. Names are case-sensitive, as in
// the table header; a wrong case or a near miss gets a suggestion. Duplicates
// are dropped.
func ResolveColumns(names []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !isKnownColumn(name) {
			if suggestion := suggestColumn(name); suggestion != "" {
				return nil, fmt.Errorf("unknown column %q; did you mean %q? (see --list-columns)", name, suggestion)
			}
			return nil, fmt.Errorf("unknown column %q (see --list-columns)", name)
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--columns needs at least one column name (see --list-columns)")
	}
	return out, nil
}

func isKnownColumn(name string) bool {
	if isNodeLagColumn(name) {
		return true
	}
	for _, def := range metricRegistry {
		if def.Column == name {
			return true
		}
	}
	return false
}

// suggestColumn returns the closest registry column name, or "" when nothing
// is close.
func suggestColumn(name string) string {
	lower := strings.ToLower(name)
	best, bestDistance := "", 1<<30
	for _, def := range metricRegistry {
		candidate := def.Column
		if strings.ToLower(candidate) == lower {
			return candidate
		}
		if d := editDistance(lower, strings.ToLower(candidate)); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	if bestDistance <= 2 || (bestDistance <= 3 && len(name) >= 8) {
		return best
	}
	return ""
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// ColumnsReadPlan says which views' FTDC paths --columns needs. The reader
// loads the summary paths plus, for each listed view, its verbose paths.
type ColumnsReadPlan struct {
	Views    []string
	Pressure bool
}

// ReadPlanForColumns maps each column to the view that derives it.
func ReadPlanForColumns(columns []string) ColumnsReadPlan {
	views := map[string]bool{}
	var plan ColumnsReadPlan
	for _, column := range columns {
		switch columnLayoutSection(column) {
		case "replication", "router":
			views["repl"] = true
		case "server":
			views["server"] = true
		case "network":
			views["network"] = true
		case "system", "io":
			views["system"] = true
		case "pressure":
			views["system"] = true
			plan.Pressure = true
		case "wiredTiger", "connPool":
			views["wt"] = true
		}
	}
	for view := range views {
		plan.Views = append(plan.Views, view)
	}
	sort.Strings(plan.Views)
	return plan
}
