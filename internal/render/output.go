package render

// OutputMode identifies whether rendering requires all rows buffered first.
type OutputMode int

const (
	// OutputTable streams rows as they are produced.
	OutputTable OutputMode = iota
	// OutputBufferedTable buffers rows before emitting a table.
	OutputBufferedTable
	// OutputJSON buffers rows before emitting the JSON document.
	OutputJSON
)

func OutputModeFor(opts Options) OutputMode {
	if opts.JSON {
		return OutputJSON
	}
	if opts.View == "io" {
		return OutputBufferedTable
	}
	return OutputTable
}

// NeedsBufferedRows reports whether the selected output mode requires
// accumulating all derived rows before rendering.
func NeedsBufferedRows(opts Options) bool {
	return OutputModeFor(opts) != OutputTable
}
