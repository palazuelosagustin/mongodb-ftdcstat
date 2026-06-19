package aggregate

import (
	"testing"
	"time"

	"ftdcstat/internal/derive"
)

func TestAverageRowsAlignsBucketsToUTCWallClock(t *testing.T) {
	rows := []derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{"activeConn": 10.0}},
		{Time: time.Date(2026, 6, 18, 19, 1, 0, 0, time.UTC), Values: map[string]any{"activeConn": 20.0}},
		{Time: time.Date(2026, 6, 18, 19, 4, 59, 0, time.UTC), Values: map[string]any{"activeConn": 30.0}},
		{Time: time.Date(2026, 6, 18, 19, 5, 0, 0, time.UTC), Values: map[string]any{"activeConn": 40.0}},
	}

	got := AverageRows(rows, 5*time.Minute)
	if len(got) != 2 {
		t.Fatalf("rows=%d", len(got))
	}
	if !got[0].Time.Equal(time.Date(2026, 6, 18, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("bucket0=%s", got[0].Time.UTC().Format(time.RFC3339))
	}
	if !got[1].Time.Equal(time.Date(2026, 6, 18, 19, 5, 0, 0, time.UTC)) {
		t.Fatalf("bucket1=%s", got[1].Time.UTC().Format(time.RFC3339))
	}
}

func TestAverageRowsAveragesNumericValues(t *testing.T) {
	rows := []derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{"activeConn": 10.0}},
		{Time: time.Date(2026, 6, 18, 19, 0, 30, 0, time.UTC), Values: map[string]any{"activeConn": 20.0}},
		{Time: time.Date(2026, 6, 18, 19, 0, 59, 0, time.UTC), Values: map[string]any{}},
	}

	got := AverageRows(rows, time.Minute)
	if len(got) != 1 {
		t.Fatalf("rows=%d", len(got))
	}
	if got[0].Values["activeConn"] != 15.0 {
		t.Fatalf("activeConn=%v", got[0].Values["activeConn"])
	}
}

func TestAverageRowsLeavesAllMissingValuesAbsent(t *testing.T) {
	rows := []derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{}},
		{Time: time.Date(2026, 6, 18, 19, 0, 30, 0, time.UTC), Values: map[string]any{}},
	}

	got := AverageRows(rows, time.Minute)
	if len(got) != 1 {
		t.Fatalf("rows=%d", len(got))
	}
	if _, ok := got[0].Values["activeConn"]; ok {
		t.Fatalf("unexpected activeConn=%v", got[0].Values["activeConn"])
	}
}

func TestAverageRowsCollapsesStateStrings(t *testing.T) {
	same := AverageRows([]derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{"rsState": "SECONDARY"}},
		{Time: time.Date(2026, 6, 18, 19, 0, 30, 0, time.UTC), Values: map[string]any{"rsState": "SECONDARY"}},
	}, time.Minute)
	if same[0].Values["rsState"] != "SECONDARY" {
		t.Fatalf("same rsState=%v", same[0].Values["rsState"])
	}

	mixed := AverageRows([]derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{"rsState": "SECONDARY"}},
		{Time: time.Date(2026, 6, 18, 19, 0, 30, 0, time.UTC), Values: map[string]any{"rsState": "PRIMARY"}},
	}, time.Minute)
	if mixed[0].Values["rsState"] != "MIXED" {
		t.Fatalf("mixed rsState=%v", mixed[0].Values["rsState"])
	}
}

func TestAverageRowsSkipsEmptyBuckets(t *testing.T) {
	rows := []derive.Row{
		{Time: time.Date(2026, 6, 18, 19, 0, 1, 0, time.UTC), Values: map[string]any{"activeConn": 10.0}},
		{Time: time.Date(2026, 6, 18, 19, 10, 1, 0, time.UTC), Values: map[string]any{"activeConn": 20.0}},
	}

	got := AverageRows(rows, 5*time.Minute)
	if len(got) != 2 {
		t.Fatalf("rows=%d", len(got))
	}
	if !got[0].Time.Equal(time.Date(2026, 6, 18, 19, 0, 0, 0, time.UTC)) || !got[1].Time.Equal(time.Date(2026, 6, 18, 19, 10, 0, 0, time.UTC)) {
		t.Fatalf("unexpected bucket starts: %s %s", got[0].Time.UTC().Format(time.RFC3339), got[1].Time.UTC().Format(time.RFC3339))
	}
}
