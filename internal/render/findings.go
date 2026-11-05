package render

import (
	"fmt"
	"io"
	"math"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/model"
)

// Event types reported by --findings.
const (
	EventProcess = "process" // first process seen in the capture
	EventRestart = "restart"
	EventGap     = "gap"
	EventRSState = "rsState"
)

// FindingsEvent is one entry of the --findings timeline. Only the fields that
// apply to Type are set.
type FindingsEvent struct {
	Time    time.Time `json:"-"`
	TimeStr string    `json:"time"`
	Type    string    `json:"type"`
	Process string    `json:"process,omitempty"`
	PID     int64     `json:"pid,omitempty"`
	Start   string    `json:"start,omitempty"`
	Seconds float64   `json:"seconds,omitempty"`
	From    string    `json:"from,omitempty"`
	To      string    `json:"to,omitempty"`
}

// FindingsReport is the --findings result: a timeline of facts, no thresholds.
type FindingsReport struct {
	Events []FindingsEvent `json:"events"`
}

// FindingsCollector builds the --findings timeline from derived rows.
type FindingsCollector struct {
	processKind string
	events      []FindingsEvent
	lastState   string
}

func NewFindingsCollector(metadata model.Metadata) *FindingsCollector {
	kind := model.ProcessKindMongod
	if metadata.ProcessKind() == model.ProcessKindMongos {
		kind = model.ProcessKindMongos
	}
	return &FindingsCollector{processKind: kind}
}

func (c *FindingsCollector) Add(row derive.Row) {
	if row.Time.IsZero() {
		return
	}
	if row.GapSeconds > 0 {
		c.add(FindingsEvent{Time: row.Time, Type: EventGap, Seconds: row.GapSeconds})
	}
	if p := row.Process; p != nil {
		event := FindingsEvent{Time: row.Time, Type: EventProcess, Process: c.processKind, PID: p.PID}
		if p.Restart {
			event.Type = EventRestart
		}
		if !p.Start.IsZero() {
			event.Start = p.Start.UTC().Format(time.RFC3339)
		}
		c.add(event)
	}
	if state, _ := row.Values["rsState"].(string); state != "" {
		if c.lastState != "" && state != c.lastState {
			c.add(FindingsEvent{Time: row.Time, Type: EventRSState, From: c.lastState, To: state})
		}
		c.lastState = state
	}
}

func (c *FindingsCollector) add(event FindingsEvent) {
	event.TimeStr = event.Time.UTC().Format(time.RFC3339)
	c.events = append(c.events, event)
}

func (c *FindingsCollector) Report() FindingsReport {
	events := c.events
	if events == nil {
		events = []FindingsEvent{}
	}
	return FindingsReport{Events: events}
}

func renderFindingsSection(w io.Writer, report FindingsReport, loc *time.Location) {
	fmt.Fprintf(w, "Findings: %d events\n\n", len(report.Events))
	if len(report.Events) == 0 {
		fmt.Fprintln(w, "no restarts, gaps, or replica-set state changes")
		return
	}
	lines := make([][]string, 0, len(report.Events))
	for _, event := range report.Events {
		lines = append(lines, []string{formatRowTime(event.Time, loc), event.Type, findingsDetail(event)})
	}
	writeAlignedTable(w, lines, 3)
}

func findingsDetail(event FindingsEvent) string {
	switch event.Type {
	case EventProcess, EventRestart:
		pid := "-"
		if event.PID != 0 {
			pid = fmt.Sprint(event.PID)
		}
		start := event.Start
		if start == "" {
			start = "-"
		}
		return fmt.Sprintf("%s pid=%s start=%s", event.Process, pid, start)
	case EventGap:
		return formatGapDuration(event.Seconds) + " without samples, rate baseline reset"
	case EventRSState:
		return event.From + " -> " + event.To
	}
	return ""
}

// formatGapDuration prints whole seconds as a compact duration with days,
// e.g. 379 -> "6m19s", 1826664 -> "21d3h24m24s".
func formatGapDuration(seconds float64) string {
	d := time.Duration(math.Round(seconds)) * time.Second
	days := d / (24 * time.Hour)
	rest := (d % (24 * time.Hour)).String()
	if days == 0 {
		return rest
	}
	if rest == "0s" {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%s", days, rest)
}
