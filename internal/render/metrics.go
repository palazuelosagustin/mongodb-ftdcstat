package render

type metricDefinition struct {
	Section  string
	Column   string
	Format   string
	JSONName string
}

var metricRegistry = []metricDefinition{
	{Section: "server", Column: "rsState", Format: "text", JSONName: "rsState"},
	{Section: "server", Column: "conn", Format: "integer", JSONName: "conn"},
	{Section: "server", Column: "qTot", Format: "integer", JSONName: "qTot"},
	{Section: "server", Column: "ins/s", Format: "rate", JSONName: "ins/s"},
	{Section: "server", Column: "qry/s", Format: "rate", JSONName: "qry/s"},
	{Section: "server", Column: "upd/s", Format: "rate", JSONName: "upd/s"},
	{Section: "server", Column: "del/s", Format: "rate", JSONName: "del/s"},
	{Section: "server", Column: "getm/s", Format: "rate", JSONName: "getm/s"},
	{Section: "server", Column: "cmd/s", Format: "rate", JSONName: "cmd/s"},
	{Section: "server", Column: "rLatS", Format: "seconds", JSONName: "rLatS"},
	{Section: "server", Column: "wLatS", Format: "seconds", JSONName: "wLatS"},
	{Section: "server", Column: "cLatS", Format: "seconds", JSONName: "cLatS"},

	{Section: "replication", Column: "majLagS", Format: "lag", JSONName: "majLagS"},

	{Section: "system", Column: "r/s", Format: "rate", JSONName: "r/s"},
	{Section: "system", Column: "w/s", Format: "rate", JSONName: "w/s"},
	{Section: "system", Column: "rkB/s", Format: "rate", JSONName: "rkB/s"},
	{Section: "system", Column: "wkB/s", Format: "rate", JSONName: "wkB/s"},
	{Section: "system", Column: "r_awaitS", Format: "seconds", JSONName: "r_awaitS"},
	{Section: "system", Column: "w_awaitS", Format: "seconds", JSONName: "w_awaitS"},
	{Section: "system", Column: "awaitS", Format: "seconds", JSONName: "awaitS"},
	{Section: "system", Column: "aqu-sz", Format: "seconds", JSONName: "aqu-sz"},
	{Section: "system", Column: "util%", Format: "percent", JSONName: "util%"},
	{Section: "system", Column: "user_cpu%", Format: "percent", JSONName: "user_cpu%"},
	{Section: "system", Column: "system_cpu%", Format: "percent", JSONName: "system_cpu%"},
	{Section: "system", Column: "iowait%", Format: "percent", JSONName: "iowait%"},
	{Section: "system", Column: "residentMB", Format: "integer", JSONName: "residentMB"},
	{Section: "system", Column: "virtualMB", Format: "integer", JSONName: "virtualMB"},

	{Section: "wiredTiger", Column: "wtCache%", Format: "percent", JSONName: "wtCache%"},
	{Section: "wiredTiger", Column: "dirty%", Format: "percent", JSONName: "dirty%"},
	{Section: "wiredTiger", Column: "wtRdMB/s", Format: "mib", JSONName: "wtRdMB/s"},
	{Section: "wiredTiger", Column: "wtWrMB/s", Format: "mib", JSONName: "wtWrMB/s"},
	{Section: "wiredTiger", Column: "evict/s", Format: "rate", JSONName: "evict/s"},
	{Section: "wiredTiger", Column: "appEvict/s", Format: "rate", JSONName: "appEvict/s"},
	{Section: "wiredTiger", Column: "ckptMS", Format: "millis", JSONName: "ckptMS"},
	{Section: "wiredTiger", Column: "rdTkt", Format: "integer", JSONName: "rdTkt"},
	{Section: "wiredTiger", Column: "wrTkt", Format: "integer", JSONName: "wrTkt"},
}

func columnsForSection(section string) []string {
	var out []string
	for _, def := range metricRegistry {
		if def.Section == section {
			out = append(out, def.Column)
		}
	}
	return out
}

func replicationColumns(nodeLabels []string) []string {
	cols := []string{"lagSLabel"}
	cols = append(cols, nodeLabels...)
	cols = append(cols, "majLagS")
	return cols
}

func metricDefinitionForColumn(column string) (metricDefinition, bool) {
	for _, def := range metricRegistry {
		if def.Column == column {
			return def, true
		}
	}
	return metricDefinition{}, false
}

func metricFormat(column string) string {
	if isNodeLagColumn(column) {
		return "lag"
	}
	if def, ok := metricDefinitionForColumn(column); ok {
		return def.Format
	}
	return ""
}

func metricJSONName(column string) string {
	if def, ok := metricDefinitionForColumn(column); ok && def.JSONName != "" {
		return def.JSONName
	}
	return column
}
