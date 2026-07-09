package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"mongodb-ftdcstat/internal/aggregate"
	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/discovery"
	"mongodb-ftdcstat/internal/ftdc"
	"mongodb-ftdcstat/internal/model"
	"mongodb-ftdcstat/internal/render"
	"mongodb-ftdcstat/internal/webui"
)

type cliOptions struct {
	Path        string
	View        string
	Interval    int
	IntervalSet bool
	Avg         time.Duration
	Device      string
	JSON        bool
	Web         bool
	TUI         bool
	Listen      string
	Verbose     bool
	Pressure    bool
	Range       model.TimeRange
}

type captureInput struct {
	reader       ftdc.NativeReader
	files        []discovery.MetricFile
	readerOpts   ftdc.ReaderOptions
	metadata     model.Metadata
	avgBucket    time.Duration
	streamerOpts derive.Options
}

type webLinks struct {
	BaseURL string
	WebURL  string
	TUIURL  string
}

type webServer interface {
	Listen(listen string) (string, error)
	Serve() error
	Close() error
}

var newWebServer = func(dataset webui.Dataset) (webServer, error) {
	return webui.NewServer(dataset)
}

func buildWebLinks(address string) webLinks {
	host := strings.TrimSpace(address)
	scheme := "http://"
	if strings.HasPrefix(host, "http://") {
		host = strings.TrimPrefix(host, "http://")
	} else if strings.HasPrefix(host, "https://") {
		scheme = "https://"
		host = strings.TrimPrefix(host, "https://")
	}
	host = strings.TrimRight(host, "/")
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if strings.HasPrefix(host, "0.0.0.0:") {
		host = strings.Replace(host, "0.0.0.0:", "127.0.0.1:", 1)
	}
	base := scheme + host
	return webLinks{BaseURL: base, WebURL: base + "/", TUIURL: base + "/tui"}
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		printError(os.Stderr, err)
		usage(os.Stderr, commandName())
		os.Exit(2)
	}

	files, warnings, err := discovery.Discover(opts.Path)
	if err != nil {
		printError(os.Stderr, err)
		os.Exit(1)
	}
	files = discovery.FilterByTimeRange(files, opts.Range)
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}

	reader := ftdc.NewNativeReader()
	metadata, metadataWarnings, err := reader.ReadMetadataFiles(files)
	if err != nil {
		printError(os.Stderr, err)
		os.Exit(1)
	}
	readerOpts := ftdc.ReaderOptionsForKind(metadata.ProcessKind(), opts.View, opts.Verbose, opts.Pressure)
	readerOpts.TimeRange = opts.Range
	warnings = append(warnings, metadataWarnings...)
	for _, warning := range metadataWarnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}

	timeLocation := time.UTC
	renderOpts := render.Options{
		View:            opts.View,
		JSON:            opts.JSON,
		ReportPath:      opts.Path,
		IntervalSeconds: opts.Interval,
		Verbose:         opts.Verbose,
		Pressure:        opts.Pressure,
		TimeLocation:    timeLocation,
	}
	input := captureInput{
		reader:     reader,
		files:      files,
		readerOpts: readerOpts,
		metadata:   metadata,
		avgBucket:  opts.Avg,
		streamerOpts: derive.Options{
			IntervalSeconds: opts.Interval,
			GapThreshold:    time.Duration(max(60, opts.Interval*10)) * time.Second,
			Device:          opts.Device,
			Metadata:        metadata,
			TimeLocation:    timeLocation,
		},
	}

	if shouldStartHTTPServer(opts) {
		if err := runWebOutput(os.Stdout, input, warnings, renderOpts, opts); err != nil {
			printError(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if render.NeedsBufferedRows(renderOpts) {
		if err := runBufferedOutput(os.Stdout, input, warnings, renderOpts); err != nil {
			printError(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runStreamingTableOutput(os.Stdout, input, warnings, renderOpts); err != nil {
		printError(os.Stderr, err)
		os.Exit(1)
	}
}

func shouldPrintCLIHeader(opts cliOptions) bool {
	return !opts.JSON
}

func shouldPrintCLIMetrics(opts cliOptions) bool {
	return !opts.JSON && !opts.TUI
}

func shouldStartHTTPServer(opts cliOptions) bool {
	return opts.Web || opts.TUI
}

func runStreamingTableOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options) error {
	metricsRange, err := streamMetricsRange(input)
	if err != nil {
		return err
	}
	renderOpts.MetricsRange = metricsRange
	renderOpts.AvgBucket = input.avgBucket
	if err := render.RenderCLIHeader(w, input.metadata, renderOpts); err != nil {
		return err
	}
	render.RenderCLIAverageNotice(w, renderOpts.AvgBucket)
	renderer, err := render.NewStreamingRenderer(w, input.metadata, renderOpts)
	if err != nil {
		return err
	}
	streamer := derive.NewStreamer(input.streamerOpts)
	averager := aggregate.NewRowBucketAverager(input.avgBucket)
	streamWarnings, err := input.reader.StreamFiles(input.files, input.readerOpts, func(sample model.MetricSample) error {
		if row, ok := streamer.Add(sample); ok {
			for _, averaged := range averager.Add(row) {
				if err := renderer.RenderRow(averaged); err != nil {
					return err
				}
			}
		}
		return nil
	})
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	for _, averaged := range averager.Flush() {
		if err := renderer.RenderRow(averaged); err != nil {
			return err
		}
	}
	_ = warnings
	return renderer.Close()
}

func runBufferedOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options) error {
	rows, metricsRange, streamWarnings, err := collectRows(input)
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	renderOpts.MetricsRange = metricsRange
	renderOpts.AvgBucket = input.avgBucket
	renderOpts.SampleCount = len(rows)
	if renderOpts.JSON {
		return render.RenderJSON(w, input.metadata, warnings, rows, renderOpts)
	}
	return render.RenderCLITable(w, input.metadata, rows, renderOpts)
}

func runWebOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options, opts cliOptions) error {
	rows, metricsRange, streamWarnings, err := collectRows(input)
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	renderOpts.MetricsRange = metricsRange
	renderOpts.AvgBucket = input.avgBucket
	renderOpts.SampleCount = len(rows)
	dataset := webui.BuildDataset(input.metadata, append(append([]model.Warning(nil), warnings...), streamWarnings...), rows, renderOpts, webui.Options{
		View:         opts.View,
		Avg:          opts.Avg,
		RowsAveraged: opts.Avg > 0,
		TimeRange:    opts.Range,
		TimeLocation: renderOpts.TimeLocation,
	})
	if dataset.Metadata.RowCount > 5000 {
		fmt.Fprintln(os.Stderr, "warning: Large capture detected. Consider using --avg 5m or --from/--to for better browser performance.")
	}
	return serveRenderedWebOutput(w, input.metadata, warnings, rows, renderOpts, opts, dataset)
}

func serveRenderedWebOutput(w io.Writer, metadata model.Metadata, warnings []model.Warning, rows []derive.Row, renderOpts render.Options, opts cliOptions, dataset webui.Dataset) error {
	server, err := newWebServer(dataset)
	if err != nil {
		return err
	}
	address, err := server.Listen(opts.Listen)
	if err != nil {
		return err
	}
	links := buildWebLinks(address)
	if opts.Web {
		renderOpts.WebLinks.WebURL = links.WebURL
	}
	if opts.TUI {
		renderOpts.WebLinks.TUIURL = links.TUIURL
	}
	if shouldPrintCLIHeader(opts) {
		if err := render.RenderCLIHeader(w, metadata, renderOpts); err != nil {
			_ = server.Close()
			return err
		}
	}
	if shouldPrintCLIMetrics(opts) {
		render.RenderCLIAverageNotice(w, renderOpts.AvgBucket)
		if err := render.RenderCLIMetrics(w, metadata, rows, renderOpts); err != nil {
			_ = server.Close()
			return err
		}
	}
	fmt.Fprintln(w, "HTTP server is running. Press Ctrl+C to stop.")
	return server.Serve()
}

func collectRows(input captureInput) ([]derive.Row, render.MetricsRange, []model.Warning, error) {
	collector := bufferedRowCollector{}
	streamer := derive.NewStreamer(input.streamerOpts)
	averager := aggregate.NewRowBucketAverager(input.avgBucket)
	var metricsRange render.MetricsRange
	streamWarnings, err := input.reader.StreamFiles(input.files, input.readerOpts, func(sample model.MetricSample) error {
		if row, ok := streamer.Add(sample); ok {
			if metricsRange.Start.IsZero() {
				metricsRange.Start = row.Time
			}
			metricsRange.End = row.Time
			for _, averaged := range averager.Add(row) {
				collector.add(averaged)
			}
		}
		return nil
	})
	if err != nil {
		return nil, render.MetricsRange{}, streamWarnings, err
	}
	for _, averaged := range averager.Flush() {
		collector.add(averaged)
	}
	return collector.snapshot(), metricsRange, streamWarnings, nil
}

func streamMetricsRange(input captureInput) (render.MetricsRange, error) {
	var out render.MetricsRange
	streamer := derive.NewStreamer(input.streamerOpts)
	_, err := input.reader.StreamFiles(input.files, input.readerOpts, func(sample model.MetricSample) error {
		row, ok := streamer.Add(sample)
		if !ok || row.Time.IsZero() {
			return nil
		}
		if out.Start.IsZero() {
			out.Start = row.Time
		}
		out.End = row.Time
		return nil
	})
	if err != nil {
		return render.MetricsRange{}, err
	}
	return out, nil
}

type bufferedRowCollector struct {
	buffer []derive.Row
}

func (c *bufferedRowCollector) add(row derive.Row) {
	c.buffer = append(c.buffer, row)
}

func (c *bufferedRowCollector) snapshot() []derive.Row {
	return c.buffer
}

func emitWarnings(warnings []model.Warning) {
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}
}

func parseArgs(args []string) (cliOptions, error) {
	opts := cliOptions{View: "summary", Interval: 60}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			usage(os.Stdout, commandName())
			os.Exit(0)
		case arg == "--json":
			opts.JSON = true
		case arg == "--web":
			opts.Web = true
		case arg == "--tui":
			opts.TUI = true
		case arg == "--verbose":
			opts.Verbose = true
		case arg == "--pressure":
			opts.Pressure = true
		case arg == "--listen":
			i++
			if i >= len(args) {
				return opts, errors.New("--listen requires a value")
			}
			opts.Listen = args[i]
		case strings.HasPrefix(arg, "--listen="):
			opts.Listen = strings.TrimPrefix(arg, "--listen=")
		case arg == "--avg":
			i++
			if i >= len(args) {
				return opts, errors.New("--avg requires a duration, for example: --avg 5m")
			}
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return opts, errors.New("--avg duration must be between 1m and 15m")
			}
			opts.Avg = d
		case strings.HasPrefix(arg, "--avg="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--avg="))
			if err != nil {
				return opts, errors.New("--avg duration must be between 1m and 15m")
			}
			opts.Avg = d
		case arg == "--view":
			i++
			if i >= len(args) {
				return opts, errors.New("--view requires a value")
			}
			opts.View = args[i]
		case strings.HasPrefix(arg, "--view="):
			opts.View = strings.TrimPrefix(arg, "--view=")
		case arg == "--interval":
			i++
			if i >= len(args) {
				return opts, errors.New("--interval requires a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return opts, errors.New("--interval must be a positive integer")
			}
			opts.Interval = n
			opts.IntervalSet = true
		case strings.HasPrefix(arg, "--interval="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--interval="))
			if err != nil || n <= 0 {
				return opts, errors.New("--interval must be a positive integer")
			}
			opts.Interval = n
			opts.IntervalSet = true
		case arg == "--device":
			i++
			if i >= len(args) {
				return opts, errors.New("--device requires a value")
			}
			opts.Device = args[i]
		case strings.HasPrefix(arg, "--device="):
			opts.Device = strings.TrimPrefix(arg, "--device=")
		case arg == "--from":
			i++
			if i >= len(args) {
				return opts, errors.New("--from requires a value")
			}
			t, err := parseTimeArg(args[i])
			if err != nil {
				return opts, fmt.Errorf("--from: %w", err)
			}
			opts.Range.From = t
		case strings.HasPrefix(arg, "--from="):
			t, err := parseTimeArg(strings.TrimPrefix(arg, "--from="))
			if err != nil {
				return opts, fmt.Errorf("--from: %w", err)
			}
			opts.Range.From = t
		case arg == "--to":
			i++
			if i >= len(args) {
				return opts, errors.New("--to requires a value")
			}
			t, err := parseTimeArg(args[i])
			if err != nil {
				return opts, fmt.Errorf("--to: %w", err)
			}
			opts.Range.To = t
		case strings.HasPrefix(arg, "--to="):
			t, err := parseTimeArg(strings.TrimPrefix(arg, "--to="))
			if err != nil {
				return opts, fmt.Errorf("--to: %w", err)
			}
			opts.Range.To = t
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown option %s", arg)
		default:
			if opts.Path != "" {
				return opts, fmt.Errorf("unexpected argument %s", arg)
			}
			opts.Path = arg
		}
	}
	if opts.Path == "" {
		return opts, errors.New("path to diagnostic data directory is required")
	}
	if !opts.Range.From.IsZero() && !opts.Range.To.IsZero() && !opts.Range.From.Before(opts.Range.To) {
		return opts, errors.New("--from must be before --to")
	}
	if opts.View == "disk" {
		return opts, errors.New("--view disk is no longer supported; use --view system")
	}
	if opts.View == "all" {
		return opts, errors.New("--view all is no longer supported; use --view summary")
	}
	if opts.JSON && (opts.Web || opts.TUI) {
		return opts, errors.New("--json cannot be combined with --web or --tui")
	}
	if opts.Listen != "" && !(opts.Web || opts.TUI) {
		return opts, errors.New("--listen is only supported with --web or --tui")
	}
	if opts.Avg > 0 && (opts.Avg < time.Minute || opts.Avg > 15*time.Minute) {
		return opts, errors.New("--avg duration must be between 1m and 15m")
	}
	if opts.Avg > 0 && opts.IntervalSet {
		return opts, errors.New("--avg cannot be combined with --interval")
	}
	switch opts.View {
	case "server", "wt", "system", "network", "repl", "summary", "io":
	default:
		return opts, errors.New("--view must be one of server, wt, system, network, repl, summary, io")
	}
	if opts.Pressure && opts.View != "system" {
		return opts, errors.New("--pressure is only supported for --view system")
	}
	if opts.Verbose && opts.View != "repl" && opts.View != "wt" && opts.View != "system" && opts.View != "network" {
		return opts, errors.New("--verbose is only supported for --view repl, wt, system, or network")
	}
	return opts, nil
}

func parseTimeArg(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05Z07:00"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("expected ISO-8601 timestamp")
}

func usage(w *os.File, name string) {
	fmt.Fprintf(w, "usage: %s <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--avg DURATION] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--tui] [--listen ADDR] [--verbose] [--pressure]\n", name)
}

func printError(w io.Writer, err error) {
	fmt.Fprintf(w, "%s: %v\n", commandName(), err)
}

func commandName() string {
	name := strings.TrimSpace(os.Args[0])
	if name == "" {
		return "mongodb-ftdcstat"
	}
	if slash := strings.LastIndexAny(name, `/\`); slash >= 0 {
		name = name[slash+1:]
	}
	if name == "" || name == "." {
		return "mongodb-ftdcstat"
	}
	return name
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
