package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/discovery"
	"ftdcstat/internal/ftdc"
	"ftdcstat/internal/model"
	"ftdcstat/internal/render"
	"ftdcstat/internal/webui"
)

type cliOptions struct {
	Path     string
	View     string
	Interval int
	Avg      time.Duration
	Device   string
	JSON     bool
	Web      bool
	Listen   string
	Verbose  bool
	Pressure bool
	Range    model.TimeRange
}

type captureInput struct {
	reader     ftdc.NativeReader
	files      []discovery.MetricFile
	readerOpts ftdc.ReaderOptions
	metadata   model.Metadata
	streamer   *derive.Streamer
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		usage(os.Stderr)
		os.Exit(2)
	}

	files, warnings, err := discovery.Discover(opts.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		os.Exit(1)
	}
	files = discovery.FilterByTimeRange(files, opts.Range)
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}

	reader := ftdc.NewNativeReader()
	readerOpts := ftdc.ReaderOptionsFor(opts.View, opts.Verbose, opts.Pressure)
	readerOpts.TimeRange = opts.Range
	metadata, metadataWarnings, err := reader.ReadMetadataFiles(files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		os.Exit(1)
	}
	warnings = append(warnings, metadataWarnings...)
	for _, warning := range metadataWarnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}

	timeLocation := time.UTC
	renderOpts := render.Options{
		View:         opts.View,
		JSON:         opts.JSON,
		Verbose:      opts.Verbose,
		Pressure:     opts.Pressure,
		TimeLocation: timeLocation,
	}
	input := captureInput{
		reader:     reader,
		files:      files,
		readerOpts: readerOpts,
		metadata:   metadata,
		streamer: derive.NewStreamer(derive.Options{
			IntervalSeconds: opts.Interval,
			GapThreshold:    time.Duration(max(60, opts.Interval*10)) * time.Second,
			Device:          opts.Device,
			Metadata:        metadata,
			TimeLocation:    timeLocation,
		}),
	}

	if opts.Web {
		if err := runWebOutput(os.Stdout, input, warnings, renderOpts, opts); err != nil {
			fmt.Fprintln(os.Stderr, "ftdcstat:", err)
			os.Exit(1)
		}
		return
	}
	if render.NeedsBufferedRows(renderOpts) {
		if err := runBufferedOutput(os.Stdout, input, warnings, renderOpts); err != nil {
			fmt.Fprintln(os.Stderr, "ftdcstat:", err)
			os.Exit(1)
		}
		return
	}
	if err := runStreamingTableOutput(os.Stdout, input, warnings, renderOpts); err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		os.Exit(1)
	}
}

func runStreamingTableOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options) error {
	renderer, err := render.NewStreamingRenderer(w, input.metadata, renderOpts)
	if err != nil {
		return err
	}
	streamWarnings, err := input.reader.StreamFiles(input.files, input.readerOpts, func(sample model.MetricSample) error {
		if row, ok := input.streamer.Add(sample); ok {
			return renderer.RenderRow(row)
		}
		return nil
	})
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	_ = warnings
	return renderer.Close()
}

func runBufferedOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options) error {
	rows, streamWarnings, err := collectRows(input)
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	return render.RenderJSON(w, input.metadata, warnings, rows, renderOpts)
}

func runWebOutput(w io.Writer, input captureInput, warnings []model.Warning, renderOpts render.Options, opts cliOptions) error {
	rows, streamWarnings, err := collectRows(input)
	emitWarnings(streamWarnings)
	if err != nil {
		return err
	}
	dataset := webui.BuildDataset(input.metadata, append(append([]model.Warning(nil), warnings...), streamWarnings...), rows, renderOpts, webui.Options{
		View:         opts.View,
		Avg:          opts.Avg,
		TimeRange:    opts.Range,
		TimeLocation: renderOpts.TimeLocation,
	})
	if dataset.Metadata.RowCount > 5000 {
		fmt.Fprintln(os.Stderr, "warning: Large capture detected. Consider using --avg 5m or --from/--to for better browser performance.")
	}
	server, err := webui.NewServer(dataset)
	if err != nil {
		return err
	}
	address, err := server.Listen(opts.Listen)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Serving ftdcstat web UI at %s\n", address)
	return server.Serve()
}

func collectRows(input captureInput) ([]derive.Row, []model.Warning, error) {
	collector := bufferedRowCollector{}
	streamWarnings, err := input.reader.StreamFiles(input.files, input.readerOpts, func(sample model.MetricSample) error {
		if row, ok := input.streamer.Add(sample); ok {
			collector.add(row)
		}
		return nil
	})
	return collector.snapshot(), streamWarnings, err
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
			usage(os.Stdout)
			os.Exit(0)
		case arg == "--json":
			opts.JSON = true
		case arg == "--web":
			opts.Web = true
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
				return opts, errors.New("--avg requires a value")
			}
			d, err := time.ParseDuration(args[i])
			if err != nil || d <= 0 {
				return opts, errors.New("--avg must be a positive duration")
			}
			opts.Avg = d
		case strings.HasPrefix(arg, "--avg="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--avg="))
			if err != nil || d <= 0 {
				return opts, errors.New("--avg must be a positive duration")
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
		case strings.HasPrefix(arg, "--interval="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--interval="))
			if err != nil || n <= 0 {
				return opts, errors.New("--interval must be a positive integer")
			}
			opts.Interval = n
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
		opts.View = "system"
	}
	if opts.View == "all" {
		opts.View = "summary"
	}
	if opts.Web && opts.JSON {
		return opts, errors.New("--web cannot be combined with --json")
	}
	if opts.Listen != "" && !opts.Web {
		return opts, errors.New("--listen is only supported with --web")
	}
	if opts.Avg > 0 && !opts.Web {
		return opts, errors.New("--avg is only supported with --web")
	}
	switch opts.View {
	case "server", "wt", "system", "network", "repl", "summary":
	default:
		return opts, errors.New("--view must be one of server, wt, system, network, repl, summary, all")
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

func usage(w *os.File) {
	fmt.Fprintln(w, "usage: ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--avg DURATION] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--listen ADDR] [--verbose] [--pressure]")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
