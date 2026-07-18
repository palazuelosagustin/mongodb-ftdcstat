package render

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
)

type HeaderTable struct {
	Title string
	Rows  []HeaderRow
}

type HeaderRow struct {
	Field string
	Value string
}

func HeaderText(metadata model.Metadata, opts Options) string {
	var buf bytes.Buffer
	if err := RenderCLIHeader(&buf, metadata, opts); err != nil {
		return ""
	}
	return buf.String()
}

func RenderCLIHeader(w io.Writer, metadata model.Metadata, opts Options) error {
	loc := opts.TimeLocation
	if loc == nil {
		loc = time.UTC
	}
	if opts.WebLinks.WebURL != "" {
		fmt.Fprintf(w, "Web UI:  %s\n", opts.WebLinks.WebURL)
	}
	if opts.WebLinks.TUIURL != "" {
		fmt.Fprintf(w, "Web TUI: %s\n", opts.WebLinks.TUIURL)
	}
	if opts.WebLinks.WebURL != "" || opts.WebLinks.TUIURL != "" {
		fmt.Fprintln(w)
	}
	for _, table := range buildHeaderTables(metadata, opts, loc) {
		renderHeaderTable(w, table)
	}
	return nil
}

func buildHeaderTables(metadata model.Metadata, opts Options, loc *time.Location) []HeaderTable {
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	build, _ := metadata.LatestDoc("buildInfo")
	host, _ := metadata.LatestDoc("hostInfo")
	cmd, _ := metadata.LatestDoc("getCmdLineOpts")
	params, _ := metadata.LatestDoc("getParameter")
	status, _ := metadata.LatestDoc("serverStatus")

	tables := []HeaderTable{
		buildReportHeader(opts),
		buildHostHeader(metadata, host),
		buildBuildHeader(metadata, build, cmd, status),
		buildReplicaSetHeader(rsInfo),
		buildCommandLineHeader(cmd),
		buildParametersHeader(cmd, params),
	}
	var out []HeaderTable
	for _, table := range tables {
		if table.hasRows() {
			// Late-bind report times so they always reflect the final metrics range.
			if table.Title == "Report" {
				injectReportTimes(&table, opts, loc, metadata)
			}
			out = append(out, table)
		}
	}
	return out
}

func buildReportHeader(opts Options) HeaderTable {
	table := HeaderTable{Title: "Report"}
	appendHeaderRow(&table.Rows, "Path", opts.ReportPath)
	appendHeaderRow(&table.Rows, "View", opts.View)
	if opts.IntervalSeconds > 0 && opts.AvgBucket <= 0 {
		appendHeaderRow(&table.Rows, "Interval", strconv.Itoa(opts.IntervalSeconds)+"s")
	}
	if opts.AvgBucket > 0 {
		appendHeaderRow(&table.Rows, "Average", FormatAvgBucket(opts.AvgBucket))
	}
	if opts.SampleCount > 0 {
		appendHeaderRow(&table.Rows, "Samples", strconv.Itoa(opts.SampleCount))
	}
	return table
}

func injectReportTimes(table *HeaderTable, opts Options, loc *time.Location, metadata model.Metadata) {
	rows := make([]HeaderRow, 0, len(table.Rows)+3)
	rows = append(rows, table.Rows...)
	appendHeaderRow(&rows, "From", formatHeaderTime(opts.MetricsRange.Start, loc))
	appendHeaderRow(&rows, "To", formatHeaderTime(opts.MetricsRange.End, loc))
	appendHeaderRow(&rows, "Process", reportProcessKind(metadata))
	table.Rows = rows
}

func reportProcessKind(metadata model.Metadata) string {
	if metadata.ProcessKind() == model.ProcessKindMongos {
		return model.ProcessKindMongos
	}
	return model.ProcessKindMongod
}

func buildHostHeader(metadata model.Metadata, host map[string]any) HeaderTable {
	table := HeaderTable{Title: "Host"}
	appendHeaderRow(&table.Rows, "Hostname", lookupFirst(host, "system.hostname", "hostname"))
	appendHeaderRow(&table.Rows, "OS", hostOS(host))
	appendHeaderRow(&table.Rows, "Kernel", lookupFirst(host, "kernelVersion", "system.kernelVersion", "extra.kernelVersion"))
	appendHeaderRow(&table.Rows, "Architecture", lookupFirst(host, "system.cpuArch", "cpuArch", "extra.cpuArch"))
	appendHeaderRow(&table.Rows, "CPU cores", lookupFirst(host, "system.numCores", "numCores", "extra.numCores"))
	appendHeaderRow(&table.Rows, "Memory", memoryDisplay(host))
	appendHeaderRow(&table.Rows, "Max connections", metadata.NetworkMaxConnDisplay())
	return table
}

func buildBuildHeader(metadata model.Metadata, build, cmd, status map[string]any) HeaderTable {
	table := HeaderTable{Title: "Build"}
	appendHeaderRow(&table.Rows, "Version", lookupString(build, "version"))
	appendHeaderRow(&table.Rows, "Git version", lookupString(build, "gitVersion"))
	appendHeaderRow(&table.Rows, "Modules", lookupList(build, "modules"))
	appendHeaderRow(&table.Rows, "Storage engine", firstString(metadata.StorageEngineName(), lookupString(status, "storageEngine.name"), lookupString(cmd, "parsed.storage.engine")))
	appendHeaderRow(&table.Rows, "Allocator", lookupString(build, "allocator"))
	appendHeaderRow(&table.Rows, "OpenSSL", lookupString(build, "openssl.running"))
	appendHeaderRow(&table.Rows, "Percona features", lookupUniqueList(build, "perconaFeatures"))
	return table
}

func buildReplicaSetHeader(info derive.ReplSetInfo) HeaderTable {
	table := HeaderTable{Title: "Replica Set"}
	appendHeaderRow(&table.Rows, "Set name", info.Set)
	if len(info.Members) > 0 {
		appendHeaderRow(&table.Rows, "Member count", strconv.Itoa(len(info.Members)))
	}
	for i, member := range info.Members {
		appendHeaderRow(&table.Rows, fmt.Sprintf("Node %d", i+1), member.Name)
	}
	return table
}

func buildCommandLineHeader(cmd map[string]any) HeaderTable {
	table := HeaderTable{Title: "Command Line Options"}
	for _, item := range parsedCmdLineItems(cmd) {
		field, value := splitKeyValue(item)
		appendHeaderRow(&table.Rows, field, value)
	}
	return table
}

func buildParametersHeader(cmd, params map[string]any) HeaderTable {
	table := HeaderTable{Title: "Parameters"}
	appendHeaderRow(&table.Rows, "wtCache", firstString(lookupString(params, "wiredTigerEngineRuntimeConfig.cache_size"), lookupString(cmd, "parsed.storage.wiredTiger.engineConfig.cacheSizeGB")))
	for _, item := range explicitParameters(cmd) {
		field, value := splitKeyValue(item)
		appendHeaderRow(&table.Rows, field, value)
	}
	return table
}

func renderHeaderTable(w io.Writer, table HeaderTable) {
	if !table.hasRows() {
		return
	}
	fieldWidth := len("Field")
	valueWidth := len("Value")
	for _, row := range table.Rows {
		if len(row.Field) > fieldWidth {
			fieldWidth = len(row.Field)
		}
		if len(row.Value) > valueWidth {
			valueWidth = len(row.Value)
		}
	}
	border := "+" + strings.Repeat("-", fieldWidth+2) + "+" + strings.Repeat("-", valueWidth+2) + "+"
	fmt.Fprintln(w, table.Title)
	fmt.Fprintln(w, border)
	fmt.Fprintf(w, "| %-*s | %-*s |\n", fieldWidth, "Field", valueWidth, "Value")
	fmt.Fprintln(w, border)
	for _, row := range table.Rows {
		fmt.Fprintf(w, "| %-*s | %-*s |\n", fieldWidth, row.Field, valueWidth, row.Value)
	}
	fmt.Fprintln(w, border)
	fmt.Fprintln(w)
}

func appendHeaderRow(rows *[]HeaderRow, field, value string) {
	value = strings.TrimSpace(value)
	if field == "" || value == "" || value == "-" {
		return
	}
	*rows = append(*rows, HeaderRow{Field: field, Value: value})
}

func (t HeaderTable) hasRows() bool {
	return len(t.Rows) > 0
}

func formatHeaderTime(ts time.Time, loc *time.Location) string {
	if ts.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	return ts.In(loc).Format(time.RFC3339)
}

func memoryDisplay(host map[string]any) string {
	memoryMB := lookupFirst(host, "system.memSizeMB", "memSizeMB", "extra.memSizeMB")
	if memoryMB == "" || memoryMB == "-" {
		return ""
	}
	mb, err := strconv.ParseFloat(memoryMB, 64)
	if err != nil || mb <= 0 {
		return memoryMB + " MiB"
	}
	if mb >= 1024 {
		return fmt.Sprintf("%.0f GiB", mb/1024)
	}
	return fmt.Sprintf("%.0f MiB", mb)
}

func splitKeyValue(item string) (string, string) {
	parts := strings.SplitN(item, "=", 2)
	if len(parts) != 2 {
		return item, "true"
	}
	return parts[0], parts[1]
}
