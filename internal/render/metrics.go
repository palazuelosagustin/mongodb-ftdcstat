package render

type metricDefinition struct {
	Section      string
	Column       string
	Format       string
	JSONName     string
	VerboseOnly  bool
	PressureOnly bool
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
	{Section: "replication", Column: "hbMs", Format: "millis", JSONName: "hbMs"},
	{Section: "replication", Column: "applyOps/s", Format: "rate", JSONName: "applyOps/s"},
	{Section: "replication", Column: "applyBufCnt", Format: "integer", JSONName: "applyBufCnt"},
	{Section: "replication", Column: "applyBufMB", Format: "mib", JSONName: "applyBufMB"},

	{Section: "system", Column: "r/s", Format: "rate", JSONName: "r/s"},
	{Section: "system", Column: "w/s", Format: "rate", JSONName: "w/s"},
	{Section: "system", Column: "rkB/s", Format: "rate", JSONName: "rkB/s", VerboseOnly: true},
	{Section: "system", Column: "wkB/s", Format: "rate", JSONName: "wkB/s", VerboseOnly: true},
	{Section: "system", Column: "awaitS", Format: "seconds", JSONName: "awaitS"},
	{Section: "system", Column: "r_awaitS", Format: "seconds", JSONName: "r_awaitS"},
	{Section: "system", Column: "w_awaitS", Format: "seconds", JSONName: "w_awaitS"},
	{Section: "system", Column: "aqu-sz", Format: "seconds", JSONName: "aqu-sz"},
	{Section: "system", Column: "util%", Format: "percent", JSONName: "util%"},
	{Section: "system", Column: "user_cpu%", Format: "percent", JSONName: "user_cpu%"},
	{Section: "system", Column: "system_cpu%", Format: "percent", JSONName: "system_cpu%"},
	{Section: "system", Column: "iowait%", Format: "percent", JSONName: "iowait%"},
	{Section: "system", Column: "residentMB", Format: "integer", JSONName: "residentMB"},
	{Section: "system", Column: "virtualMB", Format: "integer", JSONName: "virtualMB"},
	{Section: "system", Column: "ctxt/s", Format: "rate", JSONName: "ctxt/s", VerboseOnly: true},
	{Section: "system", Column: "swapIn/s", Format: "rate", JSONName: "swapIn/s", VerboseOnly: true},
	{Section: "system", Column: "swapOut/s", Format: "rate", JSONName: "swapOut/s", VerboseOnly: true},
	{Section: "system", Column: "psiCpuSome%", Format: "percent", JSONName: "psiCpuSome%", PressureOnly: true},
	{Section: "system", Column: "psiMemSome%", Format: "percent", JSONName: "psiMemSome%", PressureOnly: true},
	{Section: "system", Column: "psiMemFull%", Format: "percent", JSONName: "psiMemFull%", PressureOnly: true},
	{Section: "system", Column: "psiIoSome%", Format: "percent", JSONName: "psiIoSome%", PressureOnly: true},
	{Section: "system", Column: "psiIoFull%", Format: "percent", JSONName: "psiIoFull%", PressureOnly: true},

	{Section: "network", Column: "activeConn", Format: "integer", JSONName: "activeConn"},
	{Section: "network", Column: "idleConn", Format: "integer", JSONName: "idleConn"},
	{Section: "network", Column: "totalCreated/s", Format: "rate", JSONName: "totalCreated/s"},
	{Section: "network", Column: "queuedConn", Format: "integer", JSONName: "queuedConn", VerboseOnly: true},
	{Section: "network", Column: "rejConn/s", Format: "rate", JSONName: "rejConn/s", VerboseOnly: true},
	{Section: "network", Column: "dnsSlow/s", Format: "rate", JSONName: "dnsSlow/s", VerboseOnly: true},
	{Section: "network", Column: "tlsSlow/s", Format: "rate", JSONName: "tlsSlow/s", VerboseOnly: true},
	{Section: "network", Column: "netTimeout/s", Format: "rate", JSONName: "netTimeout/s", VerboseOnly: true},

	{Section: "wiredTiger", Column: "wtCache%", Format: "percent", JSONName: "wtCache%"},
	{Section: "wiredTiger", Column: "dirty%", Format: "percent", JSONName: "dirty%"},
	{Section: "wiredTiger", Column: "cacheMB", Format: "integer", JSONName: "cacheMB", VerboseOnly: true},
	{Section: "wiredTiger", Column: "dirtyMB", Format: "integer", JSONName: "dirtyMB", VerboseOnly: true},
	{Section: "wiredTiger", Column: "updatesMB", Format: "integer", JSONName: "updatesMB", VerboseOnly: true},
	{Section: "wiredTiger", Column: "wtRdMB/s", Format: "mib", JSONName: "wtRdMB/s"},
	{Section: "wiredTiger", Column: "wtWrMB/s", Format: "mib", JSONName: "wtWrMB/s"},
	{Section: "wiredTiger", Column: "evict/s", Format: "rate", JSONName: "evict/s"},
	{Section: "wiredTiger", Column: "appEvict/s", Format: "rate", JSONName: "appEvict/s"},
	{Section: "wiredTiger", Column: "evictWalks/s", Format: "rate", JSONName: "evictWalks/s", VerboseOnly: true},
	{Section: "wiredTiger", Column: "evictBusy/s", Format: "rate", JSONName: "evictBusy/s", VerboseOnly: true},
	{Section: "wiredTiger", Column: "ckptMS", Format: "millis", JSONName: "ckptMS"},
	{Section: "wiredTiger", Column: "ckptPages/s", Format: "rate", JSONName: "ckptPages/s", VerboseOnly: true},
	{Section: "wiredTiger", Column: "rdTkt", Format: "integer", JSONName: "rdTkt"},
	{Section: "wiredTiger", Column: "wrTkt", Format: "integer", JSONName: "wrTkt"},
	{Section: "wiredTiger", Column: "hsInsert/s", Format: "rate", JSONName: "hsInsert/s", VerboseOnly: true},
	{Section: "wiredTiger", Column: "hsRead/s", Format: "rate", JSONName: "hsRead/s", VerboseOnly: true},
	{Section: "wiredTiger", Column: "hsWriteMB/s", Format: "mib", JSONName: "hsWriteMB/s", VerboseOnly: true},
}

func columnsForSection(section string) []string {
	var out []string
	for _, def := range metricRegistry {
		if def.Section == section && !def.VerboseOnly && !def.PressureOnly {
			out = append(out, def.Column)
		}
	}
	return out
}

func wiredTigerColumns(verbose bool) []string {
	if !verbose {
		return columnsForSection("wiredTiger")
	}
	return []string{
		"wtCache%", "dirty%", "cacheMB", "dirtyMB", "updatesMB",
		"wtRdMB/s", "wtWrMB/s", "evict/s", "appEvict/s", "evictWalks/s", "evictBusy/s",
		"ckptMS", "ckptPages/s", "rdTkt", "wrTkt", "hsInsert/s", "hsRead/s", "hsWriteMB/s",
	}
}

func systemColumns(verbose bool) []string {
	cols := []string{"r/s", "w/s"}
	if verbose {
		cols = append(cols, "rkB/s", "wkB/s")
	}
	cols = append(cols,
		"awaitS", "r_awaitS", "w_awaitS", "aqu-sz", "util%",
		"user_cpu%", "system_cpu%", "iowait%", "residentMB", "virtualMB",
	)
	if verbose {
		cols = append(cols, "ctxt/s", "swapIn/s", "swapOut/s")
	}
	return cols
}

func pressureColumns() []string {
	return []string{
		"psiCpuSome%", "psiMemSome%", "psiMemFull%", "psiIoSome%", "psiIoFull%",
	}
}

func networkColumns(verbose bool) []string {
	cols := []string{"activeConn", "idleConn", "totalCreated/s"}
	if verbose {
		cols = append(cols, "queuedConn", "rejConn/s", "dnsSlow/s", "tlsSlow/s", "netTimeout/s")
	}
	return cols
}

func replicationColumns(nodeLabels []string, verbose bool) []string {
	cols := []string{"lagSLabel"}
	cols = append(cols, nodeLabels...)
	cols = append(cols, "majLagS")
	if verbose {
		cols = append(cols, "hbMs", "applyOps/s", "applyBufCnt", "applyBufMB")
	}
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
