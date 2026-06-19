package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
)

type Options struct {
	View         string
	JSON         bool
	WebURL       string
	Verbose      bool
	Pressure     bool
	TimeLocation *time.Location
}

const headerRepeatRows = 50

type tableSection struct {
	Name       string
	Start, End int
}

type tableLayout struct {
	Columns  []string
	Sections []tableSection
}

type StreamingRenderer struct {
	w                io.Writer
	cols             []string
	sections         []tableSection
	widths           []int
	separators       map[int]bool
	header           []string
	loc              *time.Location
	headerRepeatRows int
	dataRows         int
}

func Render(w io.Writer, metadata model.Metadata, warnings []model.Warning, rows []derive.Row, opts Options) error {
	if NeedsBufferedRows(opts) {
		return RenderJSON(w, metadata, warnings, rows, opts)
	}
	return renderTableRows(w, metadata, rows, opts)
}

func RenderJSON(w io.Writer, metadata model.Metadata, warnings []model.Warning, rows []derive.Row, opts Options) error {
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	layout := layoutForView(opts.View, replicationNodeLabels(rsInfo, rows), opts.Verbose, opts.Pressure)
	payload := map[string]any{
		"metadata": metadata.Summary(),
		"rsInfo":   rsInfoForJSON(rsInfo),
		"warnings": warnings,
		"view":     opts.View,
		"rows":     rowsForJSON(rows, layout),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func renderTableRows(w io.Writer, metadata model.Metadata, rows []derive.Row, opts Options) error {
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	nodeLabels := replicationNodeLabels(rsInfo, rows)
	layout := layoutForView(opts.View, nodeLabels, opts.Verbose, opts.Pressure)
	loc := opts.TimeLocation
	if loc == nil {
		loc = time.UTC
	}
	renderHeader(w, metadata, rsInfo, loc, opts.WebURL)
	renderer := newStreamingRenderer(w, layout.Columns, layout.Sections, loc)
	for _, row := range rows {
		if err := renderer.RenderRow(row); err != nil {
			return err
		}
	}
	return renderer.Close()
}

func layoutForView(view string, nodeLabels []string, verbose, pressure bool) tableLayout {
	replVerbose := verbose && view == "repl"
	switch view {
	case "server":
		return buildLayout(nil, []namedColumns{
			{Name: "server", Columns: columnsForSection("server")},
		})
	case "wt":
		return buildLayout(nil, []namedColumns{
			{Name: "wiredTiger", Columns: wiredTigerColumns(verbose)},
		})
	case "network":
		return buildLayout(nil, []namedColumns{
			{Name: "network", Columns: networkColumns(verbose)},
		})
	case "system", "disk":
		sections := []namedColumns{{Name: "system", Columns: systemColumns(verbose)}}
		if pressure {
			sections = append(sections, namedColumns{Name: "pressure", Columns: pressureColumns()})
		}
		return buildLayout(nil, sections)
	case "repl":
		return buildLayout(replicationColumns(nodeLabels, replVerbose), nil)
	case "summary", "all":
		return buildLayout(replicationColumns(nodeLabels, false), []namedColumns{
			{Name: "server", Columns: columnsForSection("server")},
			{Name: "network", Columns: columnsForSection("network")},
			{Name: "system", Columns: columnsForSection("system")},
			{Name: "wiredTiger", Columns: columnsForSection("wiredTiger")},
		})
	default:
		return buildLayout(replicationColumns(nodeLabels, false), []namedColumns{
			{Name: "server", Columns: columnsForSection("server")},
			{Name: "network", Columns: columnsForSection("network")},
			{Name: "system", Columns: columnsForSection("system")},
			{Name: "wiredTiger", Columns: columnsForSection("wiredTiger")},
		})
	}
}

type namedColumns struct {
	Name    string
	Columns []string
}

func buildLayout(replicationCols []string, sections []namedColumns) tableLayout {
	cols := []string{"time"}
	var tableSections []tableSection
	if len(replicationCols) > 0 {
		start := len(cols)
		cols = append(cols, replicationCols...)
		tableSections = append(tableSections, tableSection{Name: "replication", Start: start, End: len(cols)})
	}
	for _, section := range sections {
		start := len(cols)
		cols = append(cols, section.Columns...)
		tableSections = append(tableSections, tableSection{Name: section.Name, Start: start, End: len(cols)})
	}
	return tableLayout{Columns: cols, Sections: tableSections}
}

func replicationNodeLabels(info derive.ReplSetInfo, rows []derive.Row) []string {
	seen := map[string]bool{}
	var labels []string
	for _, member := range info.Members {
		if member.Label == "" || seen[member.Label] {
			continue
		}
		seen[member.Label] = true
		labels = append(labels, member.Label)
	}
	var discovered []int
	for _, row := range rows {
		for key := range row.Values {
			n, ok := nodeLabelNumber(key)
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			discovered = append(discovered, n)
		}
	}
	sort.Ints(discovered)
	for _, n := range discovered {
		labels = append(labels, fmt.Sprintf("node%d", n))
	}
	return labels
}

func nodeLabelNumber(label string) (int, bool) {
	if !strings.HasPrefix(label, "node") || len(label) == len("node") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(label, "node"))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func renderHeader(w io.Writer, metadata model.Metadata, rsInfo derive.ReplSetInfo, loc *time.Location, webURL string) {
	build, _ := metadata.LatestDoc("buildInfo")
	host, _ := metadata.LatestDoc("hostInfo")
	cmd, _ := metadata.LatestDoc("getCmdLineOpts")
	params, _ := metadata.LatestDoc("getParameter")
	status, _ := metadata.LatestDoc("serverStatus")

	fmt.Fprintln(w, "buildInfo")
	buildFields := []string{
		"version=" + lookupString(build, "version"),
		"git=" + lookupString(build, "gitVersion"),
		"modules=" + lookupList(build, "modules"),
		"storage=" + firstString(metadata.StorageEngineName(), lookupString(status, "storageEngine.name"), lookupString(cmd, "parsed.storage.engine")),
		"allocator=" + lookupString(build, "allocator"),
		"openssl=" + lookupString(build, "openssl.running"),
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(buildFields, " "))
	if perconaFeatures := lookupUniqueList(build, "perconaFeatures"); perconaFeatures != "-" {
		fmt.Fprintf(w, "  perconaFeatures=%s\n", perconaFeatures)
	}
	renderRSInfo(w, rsInfo)
	renderHostInfo(w, host)
	renderCmdLineOpts(w, cmd)
	fmt.Fprintln(w, "Parameters")
	fmt.Fprintf(w, "  wtCache=%s",
		firstString(lookupString(params, "wiredTigerEngineRuntimeConfig.cache_size"), lookupString(cmd, "parsed.storage.wiredTiger.engineConfig.cacheSizeGB")),
	)
	for _, item := range explicitParameters(cmd) {
		fmt.Fprintf(w, " %s", item)
	}
	fmt.Fprintln(w)
	renderNetworkHeader(w, metadata)
	if webURL != "" {
		renderWebUIHeader(w, webURL)
	}
	fmt.Fprintln(w)
}

func renderNetworkHeader(w io.Writer, metadata model.Metadata) {
	fmt.Fprintln(w, "network")
	fmt.Fprintf(w, "  maxConn: %s\n", metadata.NetworkMaxConnDisplay())
}

func renderWebUIHeader(w io.Writer, webURL string) {
	fmt.Fprintln(w, "webUI")
	fmt.Fprintf(w, "  url: %s\n", webURL)
}

func renderHostInfo(w io.Writer, host map[string]any) {
	fmt.Fprintln(w, "hostInfo")
	printHeaderFields(w, []string{
		headerField("hostname", lookupFirst(host, "system.hostname", "hostname")),
		headerField("os", hostOS(host)),
		headerField("kernel", lookupFirst(host, "kernelVersion", "system.kernelVersion", "extra.kernelVersion")),
		headerField("libc", lookupFirst(host, "libcVersion", "system.libcVersion", "extra.libcVersion")),
		headerField("arch", lookupFirst(host, "system.cpuArch", "cpuArch", "extra.cpuArch")),
		headerField("cpuAddrSize", lookupFirst(host, "system.cpuAddrSize", "cpuAddrSize", "extra.cpuAddrSize")),
		headerField("cores", lookupFirst(host, "system.numCores", "numCores", "extra.numCores")),
		headerField("availableCores", lookupFirst(host, "system.numCoresAvailableToProcess", "numCoresAvailableToProcess", "extra.numCoresAvailableToProcess")),
		headerField("physicalCores", lookupFirst(host, "system.numPhysicalCores", "numPhysicalCores", "extra.numPhysicalCores")),
		headerField("sockets", lookupFirst(host, "system.numCpuSockets", "numCpuSockets", "extra.numCpuSockets")),
		headerField("numaNodes", lookupFirst(host, "system.numNumaNodes", "numNumaNodes", "extra.numNumaNodes")),
		headerField("numaEnabled", lookupFirst(host, "system.numaEnabled", "numaEnabled", "extra.numaEnabled")),
		headerField("memoryMB", lookupFirst(host, "system.memSizeMB", "memSizeMB", "extra.memSizeMB")),
		headerField("memLimitMB", lookupFirst(host, "system.memLimitMB", "memLimitMB", "extra.memLimitMB")),
	})
	printHeaderFields(w, []string{
		headerField("maxOpenFiles", lookupFirst(host, "maxOpenFiles", "system.maxOpenFiles", "extra.maxOpenFiles")),
		headerField("pageSize", lookupFirst(host, "pageSize", "system.pageSize", "extra.pageSize")),
		headerField("numPages", lookupFirst(host, "numPages", "system.numPages", "extra.numPages")),
		headerField("overcommit_memory", lookupFirst(host, "overcommit_memory", "system.overcommit_memory", "extra.overcommit_memory")),
	})
	printHeaderFields(w, []string{
		headerField("thp_enabled", lookupFirst(host, "thp_enabled", "system.thp_enabled", "extra.thp_enabled")),
		headerField("thp_defrag", lookupFirst(host, "thp_defrag", "system.thp_defrag", "extra.thp_defrag")),
		headerField("thp_max_ptes_none", lookupFirst(host, "thp_max_ptes_none", "system.thp_max_ptes_none", "extra.thp_max_ptes_none")),
	})
	printHeaderFields(w, []string{
		headerField("versionString", lookupFirst(host, "versionString", "system.versionString", "extra.versionString")),
	})
}

func printHeaderFields(w io.Writer, fields []string) {
	var present []string
	for _, field := range fields {
		if field != "" {
			present = append(present, field)
		}
	}
	if len(present) > 0 {
		fmt.Fprintf(w, "  %s\n", strings.Join(present, " "))
	}
}

func headerField(key, value string) string {
	if value == "" || value == "-" {
		return ""
	}
	return key + "=" + value
}

func hostOS(host map[string]any) string {
	var parts []string
	if name := lookupString(host, "os.name"); name != "-" {
		parts = append(parts, name)
	}
	if version := lookupString(host, "os.version"); version != "-" {
		parts = append(parts, version)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func renderRSInfo(w io.Writer, info derive.ReplSetInfo) {
	fmt.Fprintln(w, "rsInfo")
	set := info.Set
	if set == "" {
		set = "-"
	}
	if len(info.Members) == 0 {
		fmt.Fprintf(w, "  set=%s members=-\n", set)
		return
	}
	fmt.Fprintf(w, "  set=%s members:\n", set)
	for _, member := range info.Members {
		name := member.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(w, "    %s=%s\n", member.Label, name)
	}
}

func rsInfoForJSON(info derive.ReplSetInfo) map[string]any {
	set := info.Set
	if set == "" {
		set = "-"
	}
	members := map[string]string{}
	for _, member := range info.Members {
		name := member.Name
		if name == "" {
			name = "-"
		}
		members[member.Label] = name
	}
	return map[string]any{"set": set, "members": members}
}

func rowsForJSON(rows []derive.Row, layout tableLayout) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"time": row.Time,
		}
		if row.Kind != "" {
			item["kind"] = row.Kind
		}
		if row.Marker != "" {
			item["marker"] = row.Marker
		}
		if row.ProcessMarker != "" {
			item["processMarker"] = row.ProcessMarker
		}
		for _, section := range layout.Sections {
			if section.Name == "replication" {
				replication := map[string]any{}
				lagValues := map[string]any{}
				for i := section.Start; i < section.End && i < len(layout.Columns); i++ {
					col := layout.Columns[i]
					if col == "lagSLabel" {
						continue
					}
					if !isNodeLagColumn(col) {
						replication[metricJSONName(col)] = nil
						if value, ok := row.Values[col]; ok {
							replication[metricJSONName(col)] = value
						}
						continue
					}
					if value, ok := row.Values[col]; ok {
						lagValues[col] = value
					} else {
						lagValues[col] = nil
					}
				}
				replication["lagS"] = lagValues
				item[section.Name] = replication
				continue
			}
			values := map[string]any{}
			for i := section.Start; i < section.End && i < len(layout.Columns); i++ {
				col := layout.Columns[i]
				values[metricJSONName(col)] = nil
				if value, ok := row.Values[col]; ok {
					values[metricJSONName(col)] = value
				}
			}
			item[section.Name] = values
		}
		out = append(out, item)
	}
	return out
}

func replConfigBody(doc map[string]any) map[string]any {
	if value, ok := model.Lookup(doc, "config"); ok {
		if config, ok := value.(map[string]any); ok {
			return config
		}
	}
	return doc
}

func replConfigNodeList(config map[string]any) string {
	value, ok := model.Lookup(config, "members")
	if !ok {
		return "-"
	}
	list, ok := value.([]any)
	if !ok {
		return "-"
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		host := lookupString(member, "host")
		if host == "-" {
			host = lookupString(member, "name")
		}
		if host != "-" {
			parts = append(parts, host)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func distinctReplConfigCount(records []model.MetadataRecord) int {
	seen := map[string]bool{}
	for _, record := range records {
		key := replConfigKey(record.Doc)
		if key == "" {
			continue
		}
		seen[key] = true
	}
	return len(seen)
}

func replConfigKey(doc map[string]any) string {
	config := replConfigBody(doc)
	parts := []string{replConfigConfigLine(config)}
	parts = append(parts, replConfigMemberLines(config)...)
	parts = append(parts, replConfigSettingsLine(config))
	return strings.Join(parts, "|")
}

func replConfigConfigLine(config map[string]any) string {
	fields := make(map[string]any, len(config))
	for key, value := range config {
		if key == "members" || key == "settings" {
			continue
		}
		fields[key] = value
	}
	items := flattenConfig(fields, "config", 8)
	sort.Strings(items)
	return strings.Join(items, " ")
}

func replConfigMemberLines(config map[string]any) []string {
	value, ok := model.Lookup(config, "members")
	if !ok {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	lines := make([]string, 0, len(list))
	for i, item := range list {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		items := flattenConfig(member, fmt.Sprintf("members[%d]", i), 8)
		sort.Strings(items)
		if len(items) > 0 {
			lines = append(lines, strings.Join(items, " "))
		}
	}
	return lines
}

func replConfigSettingsLine(config map[string]any) string {
	value, ok := model.Lookup(config, "settings")
	if !ok {
		return ""
	}
	settings, ok := value.(map[string]any)
	if !ok || len(settings) == 0 {
		return ""
	}
	items := flattenConfig(settings, "settings", 8)
	if len(items) == 0 {
		return ""
	}
	sort.Strings(items)
	return strings.Join(items, " ")
}

func formatMetadataTimestamp(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return "-"
	}
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(time.RFC3339)
}

func renderTable(w io.Writer, rows []derive.Row, cols []string, sections []tableSection, loc *time.Location) {
	renderer := newStreamingRenderer(w, cols, sections, loc)
	for _, row := range rows {
		_ = renderer.RenderRow(row)
	}
	_ = renderer.Close()
}

func NewStreamingRenderer(w io.Writer, metadata model.Metadata, opts Options) (*StreamingRenderer, error) {
	if opts.JSON {
		return nil, fmt.Errorf("streaming renderer does not support JSON output")
	}
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	nodeLabels := replicationNodeLabels(rsInfo, nil)
	layout := layoutForView(opts.View, nodeLabels, opts.Verbose, opts.Pressure)
	loc := opts.TimeLocation
	if loc == nil {
		loc = time.UTC
	}
	renderHeader(w, metadata, rsInfo, loc, opts.WebURL)
	return newStreamingRenderer(w, layout.Columns, layout.Sections, loc), nil
}

func newStreamingRenderer(w io.Writer, cols []string, sections []tableSection, loc *time.Location) *StreamingRenderer {
	if loc == nil {
		loc = time.UTC
	}
	header := displayColumns(cols)
	renderer := &StreamingRenderer{
		w:                w,
		cols:             cols,
		sections:         sections,
		widths:           baseColumnWidths(cols),
		separators:       separatorsFromSections(sections),
		header:           header,
		loc:              loc,
		headerRepeatRows: headerRepeatRows,
	}
	return renderer
}

func (r *StreamingRenderer) RenderRow(row derive.Row) error {
	line := tableLineForRow(r.cols, row, r.loc)
	growColumnWidths(r.widths, line)
	if r.dataRows == 0 {
		r.printHeader()
	}
	if r.dataRows > 0 && r.dataRows%r.headerRepeatRows == 0 {
		r.printHeader()
	}
	if row.Marker != "" {
		if _, err := fmt.Fprintf(r.w, "--- %s ---\n", row.Marker); err != nil {
			return err
		}
	}
	if row.ProcessMarker != "" {
		if _, err := fmt.Fprintln(r.w, row.ProcessMarker); err != nil {
			return err
		}
	}
	printLine(r.w, line, r.cols, r.widths, r.separators, false)
	r.dataRows++
	return nil
}

func (r *StreamingRenderer) Close() error {
	return nil
}

func (r *StreamingRenderer) printHeader() {
	printGroupLine(r.w, r.widths, r.sections, r.separators)
	printLine(r.w, r.header, r.cols, r.widths, r.separators, true)
}

func tableLineForRow(cols []string, row derive.Row, loc *time.Location) []string {
	line := make([]string, len(cols))
	for i, col := range cols {
		if fixed, ok := fixedColumnValue(col, row, loc); ok {
			line[i] = fixed
			continue
		}
		line[i] = format(row.Values[col], col)
	}
	return line
}

func printGroupLine(w io.Writer, widths []int, sections []tableSection, separators map[int]bool) {
	if len(sections) == 0 || len(widths) == 0 {
		return
	}
	positions, lineLen := columnPositions(widths, separators)
	line := []byte(strings.Repeat(" ", lineLen))
	for sep := range separators {
		pipe := positions[sep] - 2
		if pipe >= 0 && pipe < len(line) {
			line[pipe] = '|'
		}
	}
	for _, section := range sections {
		start := max(section.Start, 0)
		end := min(section.End, len(widths))
		if end <= start {
			continue
		}
		startPos := positions[start]
		endPos := positions[end-1] + widths[end-1]
		span := endPos - startPos
		label := section.Name
		if len(label) > span {
			label = label[:span]
		}
		offset := startPos + (span-len(label))/2
		copy(line[offset:], label)
	}
	fmt.Fprintln(w, strings.TrimRight(string(line), " "))
}

func columnPositions(widths []int, separators map[int]bool) ([]int, int) {
	positions := make([]int, len(widths))
	lineLen := 0
	for i, width := range widths {
		if i > 0 {
			lineLen++
		}
		if separators[i] {
			lineLen += 2
		}
		positions[i] = lineLen
		lineLen += width
	}
	return positions, lineLen
}

func separatorsFromSections(sections []tableSection) map[int]bool {
	out := map[int]bool{}
	for _, section := range sections {
		if section.Start <= 0 {
			continue
		}
		out[section.Start] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func printLine(w io.Writer, line, cols []string, widths []int, separators map[int]bool, header bool) {
	for j, cell := range line {
		if j > 0 {
			fmt.Fprint(w, " ")
		}
		if separators[j] {
			fmt.Fprint(w, "| ")
		}
		if header || isTextColumn(cols[j]) {
			fmt.Fprintf(w, "%-*s", widths[j], cell)
		} else {
			fmt.Fprintf(w, "%*s", widths[j], cell)
		}
	}
	fmt.Fprintln(w)
}

func displayColumns(cols []string) []string {
	out := append([]string(nil), cols...)
	for i, col := range out {
		switch col {
		case "time":
			out[i] = "datetime"
		case "lagSLabel":
			out[i] = "lagS"
		}
	}
	return out
}

func fixedColumnValue(col string, row derive.Row, loc *time.Location) (string, bool) {
	switch col {
	case "time":
		return formatRowTime(row.Time, loc), true
	case "lagSLabel":
		return "", true
	default:
		return "", false
	}
}

func formatRowTime(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(time.RFC3339)
}

func baseColumnWidths(cols []string) []int {
	header := displayColumns(cols)
	out := make([]int, len(header))
	for i, cell := range header {
		out[i] = len(cell)
	}
	return out
}

func growColumnWidths(widths []int, line []string) {
	for i, cell := range line {
		if i >= len(widths) {
			break
		}
		if len(cell) > widths[i] {
			widths[i] = len(cell)
		}
	}
}

func format(v any, key string) string {
	if v == nil {
		return "-"
	}
	switch value := v.(type) {
	case string:
		if value == "" {
			return "-"
		}
		return value
	case int:
		return fmt.Sprint(value)
	case int64:
		return fmt.Sprint(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "-"
		}
		switch metricFormat(key) {
		case "lag":
			return fmt.Sprintf("%.1f", value)
		case "seconds":
			return fmt.Sprintf("%.3f", value)
		case "millis":
			return fmt.Sprintf("%.1f", value)
		case "rate", "percent", "mib":
			return fmt.Sprintf("%.1f", value)
		case "integer", "bool":
			return fmt.Sprintf("%.0f", value)
		}
		if value == 0 {
			return "0"
		}
		if math.Abs(value-math.Round(value)) < 0.000001 {
			return fmt.Sprintf("%.0f", value)
		}
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprint(value)
	}
}

func isLatencyOrWaitColumn(key string) bool {
	return strings.Contains(key, "Lat") || strings.Contains(key, "awaitS") || key == "aqu-sz"
}

func isRateOrPercentColumn(key string) bool {
	return strings.Contains(key, "%") ||
		strings.Contains(key, "MB/s") ||
		strings.Contains(key, "kB/s") ||
		strings.HasSuffix(key, "/s") ||
		strings.HasSuffix(key, "PSI")
}

func isIntegerFloatColumn(key string) bool {
	switch key {
	case "conn", "qTot", "qRead", "qWrite", "active", "activeR", "activeW", "avail", "connQ", "rdTkt", "wrTkt", "r", "b":
		return true
	}
	return strings.HasSuffix(key, "MB") && !strings.Contains(key, "/s")
}

func isNodeLagColumn(key string) bool {
	_, ok := nodeLabelNumber(key)
	return ok
}

func isTextColumn(col string) bool {
	switch col {
	case "time", "rsState", "lagSLabel":
		return true
	default:
		return false
	}
}

func renderCmdLineOpts(w io.Writer, cmd map[string]any) {
	if len(cmd) == 0 {
		return
	}
	fmt.Fprintln(w, "getCmdLineOpts")
	for _, item := range parsedCmdLineItems(cmd) {
		fmt.Fprintf(w, "  %s\n", item)
	}
}

func parsedCmdLineItems(cmd map[string]any) []string {
	value, ok := model.Lookup(cmd, "parsed")
	if !ok {
		return nil
	}
	parsed, ok := value.(map[string]any)
	if !ok || len(parsed) == 0 {
		return nil
	}
	filtered := make(map[string]any, len(parsed))
	for key, value := range parsed {
		if key == "setParameter" {
			continue
		}
		filtered[key] = value
	}
	items := flattenConfig(filtered, "", 4)
	sort.Strings(items)
	return items
}

func explicitParameters(cmd map[string]any) []string {
	value, ok := model.Lookup(cmd, "parsed.setParameter")
	if !ok {
		return nil
	}
	items := flattenConfig(value, "", 2)
	sort.Strings(items)
	return items
}

func flattenConfig(value any, prefix string, depth int) []string {
	if depth < 0 {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var out []string
		for _, key := range keys {
			childPrefix := key
			if prefix != "" {
				childPrefix = prefix + "." + key
			}
			out = append(out, flattenConfig(v[key], childPrefix, depth-1)...)
		}
		return out
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		if prefix == "" {
			return []string{strings.Join(parts, ",")}
		}
		return []string{prefix + "=" + strings.Join(parts, ",")}
	default:
		if prefix == "" {
			return []string{fmt.Sprint(v)}
		}
		return []string{prefix + "=" + fmt.Sprint(v)}
	}
}

func lookupStringSlice(doc map[string]any, path string) []string {
	v, ok := model.Lookup(doc, path)
	if !ok {
		return nil
	}
	switch list := v.(type) {
	case []any:
		parts := make([]string, 0, len(list))
		for _, item := range list {
			parts = append(parts, fmt.Sprint(item))
		}
		return parts
	case []string:
		return append([]string(nil), list...)
	case string:
		if list == "" {
			return nil
		}
		return []string{list}
	default:
		return []string{fmt.Sprint(v)}
	}
}

func lookupString(doc map[string]any, path string) string {
	v, ok := model.Lookup(doc, path)
	if !ok {
		return "-"
	}
	s, ok := model.AsString(v)
	if !ok || s == "" {
		return "-"
	}
	return s
}

func lookupFloat(doc map[string]any, path string) (float64, bool) {
	v, ok := model.Lookup(doc, path)
	if !ok {
		return 0, false
	}
	return model.AsFloat(v)
}

func formatWholeNumber(value float64) string {
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func lookupList(doc map[string]any, path string) string {
	v, ok := model.Lookup(doc, path)
	if !ok {
		return "-"
	}
	switch list := v.(type) {
	case []any:
		parts := make([]string, 0, len(list))
		for _, item := range list {
			parts = append(parts, fmt.Sprint(item))
		}
		if len(parts) == 0 {
			return "-"
		}
		return strings.Join(parts, ",")
	case []string:
		if len(list) == 0 {
			return "-"
		}
		return strings.Join(list, ",")
	default:
		return fmt.Sprint(v)
	}
}

func lookupUniqueList(doc map[string]any, path string) string {
	v, ok := model.Lookup(doc, path)
	if !ok {
		return "-"
	}
	var raw []string
	switch list := v.(type) {
	case []any:
		raw = make([]string, 0, len(list))
		for _, item := range list {
			raw = append(raw, fmt.Sprint(item))
		}
	case []string:
		raw = append([]string(nil), list...)
	default:
		value := fmt.Sprint(v)
		if value == "" {
			return "-"
		}
		raw = []string{value}
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func lookupFirst(doc map[string]any, paths ...string) string {
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		values = append(values, lookupString(doc, path))
	}
	return firstString(values...)
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" && value != "-" {
			return value
		}
	}
	return "-"
}

func processStart(status map[string]any, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	localValue, localOK := model.Lookup(status, "localTime")
	uptimeValue, uptimeOK := model.Lookup(status, "uptime")
	if !localOK || !uptimeOK {
		return "-"
	}
	local, ok := model.AsTime(localValue)
	if !ok {
		return "-"
	}
	uptime, ok := model.AsFloat(uptimeValue)
	if !ok {
		return "-"
	}
	return local.Add(-time.Duration(uptime) * time.Second).In(loc).Format(time.RFC3339)
}

func metadataRole(status map[string]any) string {
	for _, path := range []string{"repl.ismaster", "repl.isWritablePrimary"} {
		if v, ok := model.Lookup(status, path); ok {
			if f, ok := model.AsFloat(v); ok && f > 0 {
				return "PRIMARY"
			}
		}
	}
	if v, ok := model.Lookup(status, "repl.secondary"); ok {
		if f, ok := model.AsFloat(v); ok && f > 0 {
			return "SECONDARY"
		}
	}
	return "-"
}

func metadataPrimary(repl, status map[string]any) string {
	if members, ok := model.Lookup(repl, "members"); ok {
		if list, ok := members.([]any); ok {
			for _, item := range list {
				member, ok := item.(map[string]any)
				if !ok {
					continue
				}
				state, _ := model.Lookup(member, "stateStr")
				if stateString, _ := model.AsString(state); stateString == "PRIMARY" {
					if name, ok := model.Lookup(member, "name"); ok {
						if s, ok := model.AsString(name); ok {
							return s
						}
					}
				}
			}
		}
	}
	return lookupString(status, "repl.primary")
}

func memberList(repl map[string]any) string {
	members, ok := model.Lookup(repl, "members")
	if !ok {
		return "-"
	}
	list, ok := members.([]any)
	if !ok {
		return "-"
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := lookupString(member, "name")
		if name != "-" {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func flowControlSummary(params map[string]any) string {
	enabled := firstString(lookupString(params, "flowControl.enabled"), lookupString(params, "enableFlowControl"))
	targetLag := lookupString(params, "flowControlTargetLagSeconds")
	if enabled == "-" && targetLag == "-" {
		return "-"
	}
	if targetLag == "-" {
		return "enabled=" + enabled
	}
	return "enabled=" + enabled + ",targetLagS=" + targetLag
}
