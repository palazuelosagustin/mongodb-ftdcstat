package main

import (
	"strings"
	"testing"
)

func TestParseArgsDefaultIntervalIsSixty(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Interval != 60 {
		t.Fatalf("interval=%d", opts.Interval)
	}
	if opts.View != "summary" {
		t.Fatalf("view=%s", opts.View)
	}
}

func TestParseArgsSummaryViewAccepted(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "summary"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.View != "summary" {
		t.Fatalf("view=%s", opts.View)
	}
}

func TestParseArgsAllAliasesToSummary(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "all"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.View != "summary" {
		t.Fatalf("view=%s", opts.View)
	}
}

func TestParseArgsDiskAliasesToSystem(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "disk"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.View != "system" {
		t.Fatalf("view=%s", opts.View)
	}
}

func TestParseArgsVerbose(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "system", "--verbose"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Verbose {
		t.Fatal("expected verbose=true")
	}
}

func TestParseArgsNetworkViewAccepted(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "network"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.View != "network" {
		t.Fatalf("view=%s", opts.View)
	}
}

func TestParseArgsNetworkVerbose(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "network", "--verbose"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Verbose || opts.View != "network" {
		t.Fatalf("view=%s verbose=%v", opts.View, opts.Verbose)
	}
}

func TestParseArgsPressure(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "system", "--pressure"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Pressure {
		t.Fatal("expected pressure=true")
	}
}

func TestParseArgsSystemVerbosePressure(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "system", "--verbose", "--pressure"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Verbose || !opts.Pressure {
		t.Fatalf("verbose=%v pressure=%v", opts.Verbose, opts.Pressure)
	}
}

func TestParseArgsPressureRequiresSystemView(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--view", "all", "--pressure"})
	if err == nil || !strings.Contains(err.Error(), "--pressure is only supported for --view system") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsVerboseRequiresFocusedView(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--verbose"})
	if err == nil || !strings.Contains(err.Error(), "--verbose is only supported for --view repl, wt, system, or network") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsFromTo(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--from", "2026-06-04T19:00:00", "--to", "2026-06-04T20:00:00"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Range.From.IsZero() || opts.Range.To.IsZero() {
		t.Fatalf("range not set: %#v", opts.Range)
	}
}
