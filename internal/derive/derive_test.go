package derive

import (
	"reflect"
	"testing"
	"time"

	"ftdcstat/internal/model"
)

func testSample(ts int64, sourceIndex int, values map[string]float64) model.MetricSample {
	return model.MetricSample{
		Time:        time.Unix(ts, 0).UTC(),
		SourceIndex: sourceIndex,
		Values:      values,
	}
}

func TestMergeSamplesSortsAndDeduplicates(t *testing.T) {
	a := testSample(20, 0, map[string]float64{"x": 1})
	b := testSample(10, 0, map[string]float64{"x": 2})
	c := testSample(10, 1, map[string]float64{"x": 3})
	var warnings []model.Warning
	got := MergeSamples([]model.MetricSample{a, b, c}, &warnings)
	if len(got) != 2 {
		t.Fatalf("got %d samples", len(got))
	}
	if got[0].Values["x"] != 3 {
		t.Fatalf("duplicate timestamp did not keep newest source: %#v", got[0])
	}
}

func TestRowsCalculatesRatesAcrossFileBoundaries(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(10, 0, map[string]float64{"serverStatus.opcounters.insert": 100}),
		testSample(20, 1, map[string]float64{"serverStatus.opcounters.insert": 150}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["ins/s"]; got != float64(5) {
		t.Fatalf("ins/s=%v", got)
	}
}

func TestRowsDetectsGaps(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.opcounters.insert": 1}),
		testSample(120, 0, map[string]float64{"serverStatus.opcounters.insert": 2}),
	}, Options{IntervalSeconds: 1, GapThreshold: 60 * time.Second})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Marker == "" {
		t.Fatal("expected gap marker")
	}
	if _, ok := rows[0].Values["ins/s"]; ok {
		t.Fatal("rate should be reset across gap")
	}
}

func TestRowsHandlesCounterReset(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.opcounters.insert": 100}),
		testSample(10, 0, map[string]float64{"serverStatus.opcounters.insert": 10}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if _, ok := rows[0].Values["ins/s"]; ok {
		t.Fatal("negative counter delta should not produce a rate")
	}
}

func TestRowsDetectsProcessRestart(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.uptime": 100, "serverStatus.opcounters.insert": 100}),
		testSample(10, 0, map[string]float64{"serverStatus.uptime": 1, "serverStatus.opcounters.insert": 200}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].ProcessMarker == "" {
		t.Fatal("expected restart marker")
	}
	if _, ok := rows[0].Values["ins/s"]; ok {
		t.Fatal("rate should be reset after restart")
	}
}

func TestRowsCalculatesDiskUtilization(t *testing.T) {
	prev := testSample(0, 0, map[string]float64{
		"systemMetrics.disks.sda.reads":          100,
		"systemMetrics.disks.sda.writes":         100,
		"systemMetrics.disks.sda.read_sectors":   0,
		"systemMetrics.disks.sda.write_sectors":  0,
		"systemMetrics.disks.sda.read_time_ms":   0,
		"systemMetrics.disks.sda.write_time_ms":  0,
		"systemMetrics.disks.sda.io_time_ms":     0,
		"systemMetrics.disks.sda.io_queued_ms":   0,
		"systemMetrics.disks.sda.io_in_progress": 0,
	})
	cur := testSample(10, 0, map[string]float64{
		"systemMetrics.disks.sda.reads":          110,
		"systemMetrics.disks.sda.writes":         105,
		"systemMetrics.disks.sda.read_sectors":   80,
		"systemMetrics.disks.sda.write_sectors":  40,
		"systemMetrics.disks.sda.read_time_ms":   100,
		"systemMetrics.disks.sda.write_time_ms":  200,
		"systemMetrics.disks.sda.io_time_ms":     500,
		"systemMetrics.disks.sda.io_queued_ms":   300,
		"systemMetrics.disks.sda.io_in_progress": 1,
	})
	rows := Rows([]model.MetricSample{prev, cur}, Options{IntervalSeconds: 1})
	disks := rows[0].Values["disks"].([]map[string]any)
	disk := disks[0]
	if got := disk["r/s"]; got != float64(1) {
		t.Fatalf("r/s=%v", got)
	}
	if got := disk["wkB/s"]; got != float64(2) {
		t.Fatalf("wkB/s=%v", got)
	}
	if got := disk["util%"]; got != float64(5) {
		t.Fatalf("util=%v", got)
	}
	if got := disk["awaitS"]; got != float64(0.02) {
		t.Fatalf("awaitS=%v", got)
	}
	if got := disk["r_awaitS"]; got != float64(0.01) {
		t.Fatalf("r_awaitS=%v", got)
	}
	if got := disk["w_awaitS"]; got != float64(0.04) {
		t.Fatalf("w_awaitS=%v", got)
	}
	if got := rows[0].Values["r_awaitS"]; got != float64(0.01) {
		t.Fatalf("aggregate r_awaitS=%v", got)
	}
	if got := rows[0].Values["w_awaitS"]; got != float64(0.04) {
		t.Fatalf("aggregate w_awaitS=%v", got)
	}
	if got := rows[0].Values["awaitS"]; got != float64(0.02) {
		t.Fatalf("aggregate awaitS=%v", got)
	}
}

func TestRowsSetsProcessMarkerBeforeFirstMetricLine(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.pid": 10, "serverStatus.uptime": 10}),
		testSample(10, 0, map[string]float64{"serverStatus.pid": 10, "serverStatus.uptime": 20}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].ProcessMarker == "" {
		t.Fatal("expected process marker on first rendered row")
	}
}

func TestRowsSetsPerRowRSState(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.repl.secondary": 1}),
		testSample(10, 0, map[string]float64{"serverStatus.repl.isWritablePrimary": 1}),
	}, Options{IntervalSeconds: 1})
	if got := rows[0].Values["rsState"]; got != "PRIMARY" {
		t.Fatalf("rsState=%v", got)
	}
}

func TestRowsAllowedRSStateLabels(t *testing.T) {
	cases := map[int]string{
		1:  "PRIMARY",
		2:  "SECONDARY",
		3:  "RECOVERING",
		5:  "STARTUP2",
		7:  "ARBITER",
		99: "UNKNOWN",
	}
	for state, want := range cases {
		rows := Rows([]model.MetricSample{
			testSample(0, 0, map[string]float64{"serverStatus.repl.myState": float64(state)}),
			testSample(10, 0, map[string]float64{"serverStatus.repl.myState": float64(state)}),
		}, Options{IntervalSeconds: 1})
		if got := rows[0].Values["rsState"]; got != want {
			t.Fatalf("state %d got %v want %s", state, got, want)
		}
	}
}

func TestRowsCalculatesReplLagPerMemberFromPrimaryOptime(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 100_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 99_000,
			"replSetGetStatus.members.2.state":      2,
			"replSetGetStatus.members.2.optimeDate": 97_500,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017", "h3:27017")})
	if got := rows[0].Values["node1"]; got != float64(0) {
		t.Fatalf("primary lag=%v", got)
	}
	if got := rows[0].Values["node2"]; got != float64(1) {
		t.Fatalf("node2 lag=%v", got)
	}
	if got := rows[0].Values["node3"]; got != float64(2.5) {
		t.Fatalf("node3 lag=%v", got)
	}
	if _, ok := rows[0].Values["lagS"]; ok {
		t.Fatal("old scalar lagS should not be derived")
	}
	if _, ok := rows[0].Values["replLagS"]; ok {
		t.Fatal("old scalar replLagS should not be derived")
	}
}

func TestRowsReplLagMissingPrimaryProducesNoMemberValues(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      2,
			"replSetGetStatus.members.0.optimeDate": 100_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 99_000,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017")})
	if _, ok := rows[0].Values["node1"]; ok {
		t.Fatalf("node1 should be unavailable without a visible primary: %#v", rows[0].Values)
	}
	if _, ok := rows[0].Values["node2"]; ok {
		t.Fatalf("node2 should be unavailable without a visible primary: %#v", rows[0].Values)
	}
}

func TestRowsReplLagMissingMemberOptimeOnlyOmitsThatMember(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 100_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.2.state":      2,
			"replSetGetStatus.members.2.optimeDate": 99_000,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017", "h3:27017")})
	if got := rows[0].Values["node1"]; got != float64(0) {
		t.Fatalf("primary lag=%v", got)
	}
	if _, ok := rows[0].Values["node2"]; ok {
		t.Fatalf("node2 should be unavailable without optime: %#v", rows[0].Values)
	}
	if got := rows[0].Values["node3"]; got != float64(1) {
		t.Fatalf("node3 lag=%v", got)
	}
}

func TestRowsReplLagFallsBackToOptimeTimestamp(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":       1,
			"replSetGetStatus.members.0.optime.ts.t": 100,
			"replSetGetStatus.members.1.state":       2,
			"replSetGetStatus.members.1.optime.ts.t": 98,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017")})
	if got := rows[0].Values["node1"]; got != float64(0) {
		t.Fatalf("primary lag=%v", got)
	}
	if got := rows[0].Values["node2"]; got != float64(2) {
		t.Fatalf("node2 fallback lag=%v", got)
	}
}

func TestRowsReplLagClampsNegativeLag(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 100_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 101_000,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017")})
	if got := rows[0].Values["node2"]; got != float64(0) {
		t.Fatalf("negative lag should clamp to 0.0, got %v", got)
	}
}

func TestRowsReplLagAssignsNextLabelForLaterMember(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 100_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 99_000,
		}),
		testSample(20, 0, map[string]float64{
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 110_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 109_000,
			"replSetGetStatus.members.2.state":      2,
			"replSetGetStatus.members.2.optimeDate": 108_000,
		}),
	}, Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017")})
	if _, ok := rows[0].Values["node3"]; ok {
		t.Fatalf("earlier row should not have later member value: %#v", rows[0].Values)
	}
	if got := rows[1].Values["node3"]; got != float64(2) {
		t.Fatalf("later member lag=%v", got)
	}
}

func TestRowsCalculatesOpLatency(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"serverStatus.opLatencies.reads.latency": 1000,
			"serverStatus.opLatencies.reads.ops":     10,
		}),
		testSample(10, 0, map[string]float64{
			"serverStatus.opLatencies.reads.latency": 21000,
			"serverStatus.opLatencies.reads.ops":     20,
		}),
	}, Options{IntervalSeconds: 1})
	if got := rows[0].Values["rLatS"]; got != float64(0.002) {
		t.Fatalf("rLatS=%v", got)
	}
}

func TestRowsNormalizesProcessCPUByAvailableCores(t *testing.T) {
	m := model.NewMetadata()
	m.AddDocument(time.Unix(0, 0).UTC(), "host", map[string]any{
		"hostInfo": map[string]any{
			"system": map[string]any{
				"numCoresAvailableToProcess": 4,
			},
		},
	})
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"serverStatus.extra_info.user_time_us":   0,
			"serverStatus.extra_info.system_time_us": 0,
			"systemMetrics.cpu.iowait_ms":            0,
			"systemMetrics.cpu.idle_ms":              1000,
		}),
		testSample(10, 0, map[string]float64{
			"serverStatus.extra_info.user_time_us":   20_000_000,
			"serverStatus.extra_info.system_time_us": 10_000_000,
			"systemMetrics.cpu.user_ms":              100,
			"systemMetrics.cpu.system_ms":            100,
			"systemMetrics.cpu.iowait_ms":            100,
			"systemMetrics.cpu.idle_ms":              700,
		}),
	}, Options{IntervalSeconds: 1, Metadata: m})
	row := rows[0]
	if got := row.Values["user_cpu%"]; got != float64(50) {
		t.Fatalf("user_cpu%%=%v", got)
	}
	if got := row.Values["system_cpu%"]; got != float64(25) {
		t.Fatalf("system_cpu%%=%v", got)
	}
}

func TestStreamerMatchesRows(t *testing.T) {
	opts := Options{IntervalSeconds: 1, Metadata: replTestMetadata("h1:27017", "h2:27017")}
	samples := []model.MetricSample{
		testSample(0, 0, map[string]float64{
			"serverStatus.connections.current": 1,
			"serverStatus.opcounters.insert":   10,
			"serverStatus.repl.secondary":      1,
		}),
		testSample(5, 0, map[string]float64{
			"serverStatus.connections.current":                    2,
			"serverStatus.opcounters.insert":                      20,
			"serverStatus.repl.isWritablePrimary":                 1,
			"replSetGetStatus.members.0.state":                    1,
			"replSetGetStatus.members.0.optimeDate":               100_000,
			"replSetGetStatus.members.1.state":                    2,
			"replSetGetStatus.members.1.optimeDate":               99_000,
			"serverStatus.wiredTiger.cache.bytes read into cache": 0,
		}),
		testSample(10, 0, map[string]float64{
			"serverStatus.connections.current":      3,
			"serverStatus.opcounters.insert":        30,
			"serverStatus.repl.isWritablePrimary":   1,
			"replSetGetStatus.members.0.state":      1,
			"replSetGetStatus.members.0.optimeDate": 101_000,
			"replSetGetStatus.members.1.state":      2,
			"replSetGetStatus.members.1.optimeDate": 100_000,
		}),
	}
	want := Rows(samples, opts)
	streamer := NewStreamer(opts)
	var got []Row
	for _, sample := range samples {
		if row, ok := streamer.Add(sample); ok {
			got = append(got, row)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamed rows mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func replTestMetadata(hosts ...string) model.Metadata {
	m := model.NewMetadata()
	members := make([]any, 0, len(hosts))
	for i, host := range hosts {
		members = append(members, map[string]any{"_id": i, "host": host})
	}
	m.AddDocument(time.Unix(0, 0).UTC(), "test", map[string]any{
		"replSetGetConfig": map[string]any{
			"config": map[string]any{
				"_id":     "rs0",
				"members": members,
			},
		},
	})
	return m
}

func TestRowsHandlesMissingFields(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
}

func TestRowsCalculatesApplyRateAcrossFileBoundaries(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(1748647354, 0, map[string]float64{"serverStatus.metrics.repl.apply.ops": 1000000}),
		testSample(1748647355, 1, map[string]float64{"serverStatus.metrics.repl.apply.ops": 1000100}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["applyOps/s"]; got != float64(100) {
		t.Fatalf("applyOps/s=%v", got)
	}
}

func TestRowsResetsApplyRateAcrossLargeGap(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(1749073232, 0, map[string]float64{"serverStatus.metrics.repl.apply.ops": 1000000}),
		testSample(1749074727, 1, map[string]float64{"serverStatus.metrics.repl.apply.ops": 1000100}),
	}, Options{IntervalSeconds: 1, GapThreshold: 600 * time.Second})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Marker == "" {
		t.Fatal("expected gap marker")
	}
	if _, ok := rows[0].Values["applyOps/s"]; ok {
		t.Fatal("applyOps/s should be reset across large gap")
	}
}

func TestRowsResetsApplyRateOnCounterReset(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.metrics.repl.apply.ops": 1000000}),
		testSample(10, 0, map[string]float64{"serverStatus.metrics.repl.apply.ops": 100}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if _, ok := rows[0].Values["applyOps/s"]; ok {
		t.Fatal("applyOps/s should be absent after counter reset")
	}
}

func TestRowsResetsApplyRateOnProcessRestart(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{"serverStatus.uptime": 100, "serverStatus.metrics.repl.apply.ops": 1000000}),
		testSample(10, 0, map[string]float64{"serverStatus.uptime": 1, "serverStatus.metrics.repl.apply.ops": 1000100}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].ProcessMarker == "" {
		t.Fatal("expected restart marker")
	}
	if _, ok := rows[0].Values["applyOps/s"]; ok {
		t.Fatal("applyOps/s should be reset after restart")
	}
}

func TestRowsAverageMemberPingMs(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"replSetGetStatus.members.0.pingMs": 10,
			"replSetGetStatus.members.1.pingMs": 30,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["hbMs"]; got != float64(20) {
		t.Fatalf("hbMs=%v", got)
	}
}

func TestRowsAverageMemberPingMsMissing(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if _, ok := rows[0].Values["hbMs"]; ok {
		t.Fatal("hbMs should be absent when pingMs is missing")
	}
}

func TestRowsCalculatesNetworkMetrics(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"serverStatus.connections.current":      20,
			"serverStatus.connections.active":       8,
			"serverStatus.connections.totalCreated": 100,
			"serverStatus.connections.rejected":     5,
			"serverStatus.network.numSlowDNSOperations": 10,
			"serverStatus.network.numSlowSSLOperations": 20,
			"serverStatus.metrics.operation.numConnectionNetworkTimeouts": 30,
		}),
		testSample(10, 0, map[string]float64{
			"serverStatus.connections.current":      21,
			"serverStatus.connections.active":       9,
			"serverStatus.connections.totalCreated": 115,
			"serverStatus.connections.rejected":     7,
			"serverStatus.connections.queuedForEstablishment": 3,
			"serverStatus.network.numSlowDNSOperations": 14,
			"serverStatus.network.numSlowSSLOperations": 25,
			"serverStatus.metrics.operation.numConnectionNetworkTimeouts": 31,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["activeConn"]; got != float64(9) {
		t.Fatalf("activeConn=%v", got)
	}
	if got := rows[0].Values["idleConn"]; got != float64(12) {
		t.Fatalf("idleConn=%v", got)
	}
	for key, want := range map[string]float64{
		"totalCreated/s": 1.5,
		"queuedConn":     3,
		"rejConn/s":      0.2,
		"dnsSlow/s":      0.4,
		"tlsSlow/s":      0.5,
		"netTimeout/s":   0.1,
	} {
		if got := rows[0].Values[key]; got != want {
			t.Fatalf("%s=%v want %v", key, got, want)
		}
	}
}

func TestRowsClampsIdleConnectionsToZero(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"serverStatus.connections.current": 5,
			"serverStatus.connections.active":  7,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["idleConn"]; got != float64(0) {
		t.Fatalf("idleConn=%v", got)
	}
}

func TestRowsResetsNetworkRatesAcrossGapAndRestart(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"serverStatus.uptime":                  100,
			"serverStatus.connections.totalCreated": 100,
		}),
		testSample(120, 0, map[string]float64{
			"serverStatus.uptime":                  1,
			"serverStatus.connections.totalCreated": 110,
		}),
	}, Options{IntervalSeconds: 1, GapThreshold: 60 * time.Second})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Marker == "" || rows[0].ProcessMarker == "" {
		t.Fatalf("expected gap and restart markers: %#v", rows[0])
	}
	if _, ok := rows[0].Values["totalCreated/s"]; ok {
		t.Fatal("totalCreated/s should be reset across gap/restart")
	}
}

func TestRowsApplyBufferGauges(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{}),
		testSample(10, 0, map[string]float64{
			"serverStatus.metrics.repl.buffer.apply.count":     42,
			"serverStatus.metrics.repl.buffer.apply.sizeBytes": 2 * 1024 * 1024,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["applyBufCnt"]; got != float64(42) {
		t.Fatalf("applyBufCnt=%v", got)
	}
	if got := rows[0].Values["applyBufMB"]; got != float64(2) {
		t.Fatalf("applyBufMB=%v", got)
	}
}

func TestRowsCalculatesVerboseSystemMetrics(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"systemMetrics.cpu.ctxt":      1000,
			"systemMetrics.vmstat.pswpin":   10,
			"systemMetrics.vmstat.pswpout":  20,
		}),
		testSample(10, 0, map[string]float64{
			"systemMetrics.cpu.ctxt":      1200,
			"systemMetrics.vmstat.pswpin":   20,
			"systemMetrics.vmstat.pswpout":  50,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	values := rows[0].Values
	want := map[string]float64{
		"ctxt/s":    20,
		"swapIn/s":  1,
		"swapOut/s": 3,
	}
	for key, wantValue := range want {
		if got := values[key]; got != wantValue {
			t.Fatalf("%s=%v want %v", key, got, wantValue)
		}
	}
}

func TestRowsCalculatesPressureSystemMetrics(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"systemMetrics.pressure.cpu.some.totalMicros":    1000000,
			"systemMetrics.pressure.memory.some.totalMicros": 2000000,
			"systemMetrics.pressure.memory.full.totalMicros": 3000000,
			"systemMetrics.pressure.io.some.totalMicros":     4000000,
			"systemMetrics.pressure.io.full.totalMicros":     5000000,
		}),
		testSample(10, 0, map[string]float64{
			"systemMetrics.pressure.cpu.some.totalMicros":    2000000,
			"systemMetrics.pressure.memory.some.totalMicros": 4000000,
			"systemMetrics.pressure.memory.full.totalMicros": 6000000,
			"systemMetrics.pressure.io.some.totalMicros":     8000000,
			"systemMetrics.pressure.io.full.totalMicros":     10000000,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	values := rows[0].Values
	want := map[string]float64{
		"psiCpuSome%":  10,
		"psiMemSome%":  20,
		"psiMemFull%":  30,
		"psiIoSome%":   40,
		"psiIoFull%":   50,
	}
	for key, wantValue := range want {
		if got := values[key]; got != wantValue {
			t.Fatalf("%s=%v want %v", key, got, wantValue)
		}
	}
}

func TestRowsCalculatesPressureFromAvg10(t *testing.T) {
	rows := Rows([]model.MetricSample{
		testSample(0, 0, map[string]float64{
			"systemMetrics.pressure.cpu.some.avg10": 12.5,
		}),
		testSample(10, 0, map[string]float64{
			"systemMetrics.pressure.cpu.some.avg10": 15.0,
		}),
	}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if got := rows[0].Values["psiCpuSome%"]; got != 15.0 {
		t.Fatalf("psiCpuSome%%=%v want 15", got)
	}
}

func TestRowsCalculatesVerboseWiredTigerMetrics(t *testing.T) {
	prev := testSample(0, 0, map[string]float64{
		"serverStatus.wiredTiger.cache.bytes currently in the cache":                                                        256 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.maximum bytes configured":                                                            1024 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.tracked dirty bytes in the cache":                                                    32 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes belonging to the updates in the cache":                                         16 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes read into cache":                                                               100 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes written from cache":                                                            200 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.eviction walks started from root of tree":                                            10,
		"serverStatus.wiredTiger.cache.eviction walks started from saved location in tree":                                  20,
		"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted":                                    30,
		"serverStatus.wiredTiger.transaction.transaction checkpoint pages written":                                          100,
		"serverStatus.wiredTiger.cache.history store table insert calls":                                                    200,
		"serverStatus.wiredTiger.cache.history store table read calls":                                                      300,
		"serverStatus.wiredTiger.cache.bytes written from cache into history store":                                         400 * 1024 * 1024,
		"serverStatus.wiredTiger.transaction.transaction checkpoint most recent duration for gathering all handles (usecs)": 1000,
	})
	cur := testSample(10, 0, map[string]float64{
		"serverStatus.wiredTiger.cache.bytes currently in the cache":                                                        512 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.maximum bytes configured":                                                            1024 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.tracked dirty bytes in the cache":                                                    64 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes belonging to the updates in the cache":                                         32 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes read into cache":                                                               120 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.bytes written from cache":                                                            210 * 1024 * 1024,
		"serverStatus.wiredTiger.cache.eviction walks started from root of tree":                                            20,
		"serverStatus.wiredTiger.cache.eviction walks started from saved location in tree":                                  25,
		"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted":                                    40,
		"serverStatus.wiredTiger.transaction.transaction checkpoint pages written":                                          140,
		"serverStatus.wiredTiger.cache.history store table insert calls":                                                    220,
		"serverStatus.wiredTiger.cache.history store table read calls":                                                      330,
		"serverStatus.wiredTiger.cache.bytes written from cache into history store":                                         405 * 1024 * 1024,
		"serverStatus.wiredTiger.transaction.transaction checkpoint most recent duration for gathering all handles (usecs)": 2000,
	})
	rows := Rows([]model.MetricSample{prev, cur}, Options{IntervalSeconds: 1})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	values := rows[0].Values
	want := map[string]float64{
		"cacheMB":      512,
		"dirtyMB":      64,
		"updatesMB":    32,
		"wtRdMB/s":     2,
		"wtWrMB/s":     1,
		"evictWalks/s": 1.5,
		"evictBusy/s":  1,
		"ckptMS":       2,
		"ckptPages/s":  4,
		"hsInsert/s":   2,
		"hsRead/s":     3,
		"hsWriteMB/s":  0.5,
	}
	for key, wantValue := range want {
		if got := values[key]; got != wantValue {
			t.Fatalf("%s=%v want %v", key, got, wantValue)
		}
	}
}
