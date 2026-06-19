package render

import (
	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
)

type MetricInfo struct {
	Section  string `json:"section"`
	Column   string `json:"column"`
	JSONName string `json:"jsonName"`
	Format   string `json:"format"`
}

type ViewSection struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

type ViewDescription struct {
	Columns  []string      `json:"columns"`
	Sections []ViewSection `json:"sections"`
}

func DescribeView(metadata model.Metadata, rows []derive.Row, opts Options) ViewDescription {
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	nodeLabels := replicationNodeLabels(rsInfo, rows)
	layout := layoutForView(opts.View, nodeLabels, opts.Verbose, opts.Pressure)
	sections := make([]ViewSection, 0, len(layout.Sections))
	for _, section := range layout.Sections {
		if section.Start < 0 || section.End > len(layout.Columns) || section.End < section.Start {
			continue
		}
		sections = append(sections, ViewSection{
			Name:    section.Name,
			Columns: append([]string(nil), layout.Columns[section.Start:section.End]...),
		})
	}
	return ViewDescription{
		Columns:  append([]string(nil), layout.Columns...),
		Sections: sections,
	}
}

func MetricInfoForColumn(column string) MetricInfo {
	if def, ok := metricDefinitionForColumn(column); ok {
		return MetricInfo{
			Section:  def.Section,
			Column:   def.Column,
			JSONName: metricJSONName(column),
			Format:   def.Format,
		}
	}
	if isNodeLagColumn(column) {
		return MetricInfo{
			Section:  "replication",
			Column:   column,
			JSONName: column,
			Format:   "lag",
		}
	}
	return MetricInfo{
		Column:   column,
		JSONName: column,
	}
}
