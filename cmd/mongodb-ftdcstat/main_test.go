package main

import (
	"strings"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/render"
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

func TestParseArgsAllViewRejectedWithReplacementHint(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--view", "all"})
	if err == nil || !strings.Contains(err.Error(), "--view all is no longer supported; use --view summary") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseArgsDiskViewRejectedWithReplacementHint(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--view", "disk"})
	if err == nil || !strings.Contains(err.Error(), "--view disk is no longer supported; use --view system") {
		t.Fatalf("err=%v", err)
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

func TestParseArgsIOViewAccepted(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--view", "io"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.View != "io" {
		t.Fatalf("view=%s", opts.View)
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
	_, err := parseArgs([]string{"diagnostic.data", "--view", "summary", "--pressure"})
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

func TestIOViewRequiresBufferedRows(t *testing.T) {
	if !render.NeedsBufferedRows(render.Options{View: "io"}) {
		t.Fatal("io table output should buffer rows")
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

func TestParseArgsAcceptsTUIFlag(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--tui"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.TUI {
		t.Fatal("expected --tui to enable cliOptions.TUI")
	}
	if opts.Web {
		t.Fatal("expected --tui not to imply --web during phase 1")
	}
}

func TestParseArgsLeavesExistingWebFlagBehavior(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--web"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Web {
		t.Fatal("expected --web to enable cliOptions.Web")
	}
	if opts.TUI {
		t.Fatal("expected --web not to imply --tui")
	}
}

func TestParseArgsTUIListenAndAvg(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--tui", "--listen", "127.0.0.1:8080", "--avg", "5m"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.TUI {
		t.Fatal("expected tui=true")
	}
	if opts.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen=%s", opts.Listen)
	}
	if opts.Avg != 5*time.Minute {
		t.Fatalf("avg=%s", opts.Avg)
	}
}

func TestParseArgsWebAndTUIAllowedTogether(t *testing.T) {
	opts, err := parseArgs([]string{"diagnostic.data", "--web", "--tui"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Web || !opts.TUI {
		t.Fatalf("web=%v tui=%v", opts.Web, opts.TUI)
	}
}

func TestParseArgsWebOrTUIRejectsJSON(t *testing.T) {
	for _, args := range [][]string{
		{"diagnostic.data", "--web", "--json"},
		{"diagnostic.data", "--tui", "--json"},
		{"diagnostic.data", "--web", "--tui", "--json"},
	} {
		_, err := parseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "--json cannot be combined with --web or --tui") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
}

func TestParseArgsListenRequiresWebOrTUI(t *testing.T) {
	_, err := parseArgs([]string{"diagnostic.data", "--listen", "127.0.0.1:8080"})
	if err == nil || !strings.Contains(err.Error(), "--listen is only supported with --web or --tui") {
		t.Fatalf("err=%v", err)
	}
}

func TestRenderDecisionHelpers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    cliOptions
		header  bool
		metrics bool
		http    bool
	}{
		{name: "default", opts: cliOptions{}, header: true, metrics: true, http: false},
		{name: "web", opts: cliOptions{Web: true}, header: true, metrics: true, http: true},
		{name: "tui", opts: cliOptions{TUI: true}, header: true, metrics: false, http: true},
		{name: "web+tui", opts: cliOptions{Web: true, TUI: true}, header: true, metrics: false, http: true},
		{name: "json", opts: cliOptions{JSON: true}, header: false, metrics: false, http: false},
	} {
		if got := shouldPrintCLIHeader(tc.opts); got != tc.header {
			t.Fatalf("%s header: got %v want %v", tc.name, got, tc.header)
		}
		if got := shouldPrintCLIMetrics(tc.opts); got != tc.metrics {
			t.Fatalf("%s metrics: got %v want %v", tc.name, got, tc.metrics)
		}
		if got := shouldStartHTTPServer(tc.opts); got != tc.http {
			t.Fatalf("%s http: got %v want %v", tc.name, got, tc.http)
		}
	}
}

func TestBuildWebLinks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address string
		wantWeb string
		wantTUI string
	}{
		{name: "implicit localhost", address: ":8080", wantWeb: "http://127.0.0.1:8080/", wantTUI: "http://127.0.0.1:8080/tui"},
		{name: "all interfaces", address: "0.0.0.0:8080", wantWeb: "http://127.0.0.1:8080/", wantTUI: "http://127.0.0.1:8080/tui"},
		{name: "explicit localhost", address: "127.0.0.1:9090", wantWeb: "http://127.0.0.1:9090/", wantTUI: "http://127.0.0.1:9090/tui"},
		{name: "http address", address: "http://127.0.0.1:7777", wantWeb: "http://127.0.0.1:7777/", wantTUI: "http://127.0.0.1:7777/tui"},
		{name: "http all interfaces", address: "http://0.0.0.0:7777", wantWeb: "http://127.0.0.1:7777/", wantTUI: "http://127.0.0.1:7777/tui"},
	} {
		got := buildWebLinks(tc.address)
		if got.WebURL != tc.wantWeb || got.TUIURL != tc.wantTUI {
			t.Fatalf("%s: got=%+v wantWeb=%s wantTUI=%s", tc.name, got, tc.wantWeb, tc.wantTUI)
		}
	}
}
