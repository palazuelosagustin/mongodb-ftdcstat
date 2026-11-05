package render

// OutputMode identifies whether rendering requires all rows buffered first.
type OutputMode int

const (
	// OutputTable streams rows as they are produced.
	OutputTable OutputMode = iota
	// OutputBufferedTable buffers rows before emitting a table.
	OutputBufferedTable
	// OutputJSON emits the JSON document. The CLI streams it row by row
	// (JSONStream); Render, which already has all rows, writes it at once.
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
