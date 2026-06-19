package main

import (
	"strings"
	"testing"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/render"
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

func TestParseArgsWeb(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--web"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Web {
		t.Fatal("expected web=true")
	}
}

func TestParseArgsWebListenAndAvg(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--web", "--listen", "127.0.0.1:8080", "--avg", "5m"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen=%s", opts.Listen)
	}
	if opts.Avg != 5*time.Minute {
		t.Fatalf("avg=%s", opts.Avg)
	}
}

func TestParseArgsAvgValidDurations(t *testing.T) {
	for _, value := range []string{"1m", "5m", "15m"} {
		opts, err := parseArgs([]string{"diagnostic.data", "--avg", value})
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if opts.Avg <= 0 {
			t.Fatalf("%s: avg=%s", value, opts.Avg)
		}
	}
}

func TestParseArgsAvgMissingDurationFails(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--avg"})
	if err == nil || !strings.Contains(err.Error(), "--avg requires a duration, for example: --avg 5m") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsAvgDurationRangeFails(t *testing.T) {
	for _, value := range []string{"30s", "16m", "1h"} {
		_, err := parseArgs([]string{"diagnostic.data", "--avg", value})
		if err == nil || !strings.Contains(err.Error(), "--avg duration must be between 1m and 15m") {
			t.Fatalf("%s: err=%v", value, err)
		}
	}
}

func TestParseArgsAvgRejectsExplicitInterval(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--avg", "5m", "--interval", "120"})
	if err == nil || !strings.Contains(err.Error(), "--avg cannot be combined with --interval") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsWebRejectsJSON(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--web", "--json"})
	if err == nil || !strings.Contains(err.Error(), "--web cannot be combined with --json") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsListenRequiresWeb(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--listen", "127.0.0.1:8080"})
	if err == nil || !strings.Contains(err.Error(), "--listen is only supported with --web") {
		t.Fatalf("err=%v", err)
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

func TestTableOutputDoesNotRequireBufferedRows(t *testing.T) {
	if render.NeedsBufferedRows(render.Options{View: "summary"}) {
		t.Fatal("table output should stream rows")
	}
}

func TestJSONOutputRequiresBufferedRows(t *testing.T) {
	if !render.NeedsBufferedRows(render.Options{View: "summary", JSON: true}) {
		t.Fatal("json output should buffer rows")
	}
}

func TestBufferedRowCollectorOnlyUsedForJSONPath(t *testing.T) {
	collector := bufferedRowCollector{}
	collector.add(derive.Row{Time: time.Unix(0, 0)})
	if len(collector.snapshot()) != 1 {
		t.Fatalf("collector rows=%d", len(collector.snapshot()))
	}
}
