package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/discovery"
	"ftdcstat/internal/ftdc"
	"ftdcstat/internal/model"
	"ftdcstat/internal/render"
)

type cliOptions struct {
	Path     string
	View     string
	Interval int
	Device   string
	JSON     bool
	Verbose  bool
	Pressure bool
	Range    model.TimeRange
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
	streamer := derive.NewStreamer(derive.Options{
		IntervalSeconds: opts.Interval,
		GapThreshold:    time.Duration(max(60, opts.Interval*10)) * time.Second,
		Device:          opts.Device,
		Metadata:        metadata,
		TimeLocation:    timeLocation,
	})
	var rows []derive.Row
	streamWarnings, err := reader.StreamFiles(files, readerOpts, func(sample model.MetricSample) error {
		if row, ok := streamer.Add(sample); ok {
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		os.Exit(1)
	}
	warnings = append(warnings, streamWarnings...)
	for _, warning := range streamWarnings {
		fmt.Fprintln(os.Stderr, "warning:", warning.String())
	}
	if err := render.Render(os.Stdout, metadata, warnings, rows, render.Options{View: opts.View, JSON: opts.JSON, Verbose: opts.Verbose, Pressure: opts.Pressure, TimeLocation: timeLocation}); err != nil {
		fmt.Fprintln(os.Stderr, "ftdcstat:", err)
		os.Exit(1)
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
		case arg == "--verbose":
			opts.Verbose = true
		case arg == "--pressure":
			opts.Pressure = true
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
	fmt.Fprintln(w, "usage: ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--verbose] [--pressure]")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
