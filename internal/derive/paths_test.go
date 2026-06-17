package derive

import "testing"

func TestViewNeedsVerboseReplication(t *testing.T) {
	cases := []struct {
		view    string
		verbose bool
		want    bool
	}{
		{"repl", true, true},
		{"summary", true, false},
		{"server", true, false},
		{"system", true, false},
		{"repl", false, false},
		{"summary", false, false},
	}
	for _, tc := range cases {
		if got := ViewNeedsVerboseReplication(tc.view, tc.verbose); got != tc.want {
			t.Fatalf("ViewNeedsVerboseReplication(%q, %v)=%v want %v", tc.view, tc.verbose, got, tc.want)
		}
	}
}

func TestViewNeedsPressureSystem(t *testing.T) {
	cases := []struct {
		view     string
		pressure bool
		want     bool
	}{
		{"system", true, true},
		{"summary", true, false},
		{"all", true, false},
		{"system", false, false},
	}
	for _, tc := range cases {
		if got := ViewNeedsPressureSystem(tc.view, tc.pressure); got != tc.want {
			t.Fatalf("ViewNeedsPressureSystem(%q, %v)=%v want %v", tc.view, tc.pressure, got, tc.want)
		}
	}
}

func TestViewNeedsVerboseNetwork(t *testing.T) {
	cases := []struct {
		view    string
		verbose bool
		want    bool
	}{
		{"network", true, true},
		{"system", true, false},
		{"network", false, false},
	}
	for _, tc := range cases {
		if got := ViewNeedsVerboseNetwork(tc.view, tc.verbose); got != tc.want {
			t.Fatalf("ViewNeedsVerboseNetwork(%q, %v)=%v want %v", tc.view, tc.verbose, got, tc.want)
		}
	}
}

func TestRequiredPathsForVerboseReplication(t *testing.T) {
	paths, _ := RequiredPathsFor("repl", true, false)
	for _, path := range verboseReplicationPaths {
		if !paths[path] {
			t.Fatalf("expected verbose path %q", path)
		}
	}
	plain, _ := RequiredPathsFor("summary", false, false)
	for _, path := range verboseReplicationPaths {
		if plain[path] {
			t.Fatalf("non-verbose should not include %q", path)
		}
	}
	summaryVerbose, _ := RequiredPathsFor("summary", true, false)
	for _, path := range verboseReplicationPaths {
		if summaryVerbose[path] {
			t.Fatalf("summary should ignore --verbose and not include %q", path)
		}
	}
	systemVerbose, _ := RequiredPathsFor("system", true, false)
	for _, path := range verboseReplicationPaths {
		if systemVerbose[path] {
			t.Fatalf("system verbose should not include %q", path)
		}
	}
}

func TestRequiredPathsForVerboseWiredTiger(t *testing.T) {
	paths, _ := RequiredPathsFor("wt", true, false)
	for _, path := range verboseWiredTigerPaths {
		if !paths[path] {
			t.Fatalf("expected verbose WiredTiger path %q", path)
		}
	}
	plain, _ := RequiredPathsFor("wt", false, false)
	for _, path := range verboseWiredTigerPaths {
		if plain[path] {
			t.Fatalf("non-verbose wt should not include %q", path)
		}
	}
	summaryVerbose, _ := RequiredPathsFor("summary", true, false)
	for _, path := range verboseWiredTigerPaths {
		if summaryVerbose[path] {
			t.Fatalf("summary verbose should not include %q", path)
		}
	}
}

func TestRequiredPathsForVerboseSystem(t *testing.T) {
	paths, _ := RequiredPathsFor("system", true, false)
	for _, path := range verboseSystemPaths {
		if !paths[path] {
			t.Fatalf("expected verbose system path %q", path)
		}
	}
	plain, _ := RequiredPathsFor("system", false, false)
	for _, path := range verboseSystemPaths {
		if plain[path] {
			t.Fatalf("non-verbose system should not include %q", path)
		}
	}
	summaryVerbose, _ := RequiredPathsFor("summary", true, false)
	for _, path := range verboseSystemPaths {
		if summaryVerbose[path] {
			t.Fatalf("summary verbose should not include %q", path)
		}
	}
}

func TestRequiredPathsForPressureSystem(t *testing.T) {
	paths, _ := RequiredPathsFor("system", false, true)
	for _, path := range pressureSystemPaths {
		if !paths[path] {
			t.Fatalf("expected pressure system path %q", path)
		}
	}
	plain, _ := RequiredPathsFor("system", false, false)
	for _, path := range pressureSystemPaths {
		if plain[path] {
			t.Fatalf("non-pressure system should not include %q", path)
		}
	}
	summaryPressure, _ := RequiredPathsFor("summary", false, true)
	for _, path := range pressureSystemPaths {
		if summaryPressure[path] {
			t.Fatalf("summary pressure should not include %q", path)
		}
	}
}

func TestRequiredPathsForVerboseNetwork(t *testing.T) {
	paths, _ := RequiredPathsFor("network", true, false)
	for _, path := range verboseNetworkPaths {
		if !paths[path] {
			t.Fatalf("expected verbose network path %q", path)
		}
	}
	plain, _ := RequiredPathsFor("network", false, false)
	for _, path := range verboseNetworkPaths {
		if plain[path] {
			t.Fatalf("non-verbose network should not include %q", path)
		}
	}
	summaryVerbose, _ := RequiredPathsFor("summary", true, false)
	for _, path := range verboseNetworkPaths {
		if summaryVerbose[path] {
			t.Fatalf("summary verbose should not include %q", path)
		}
	}
}

func TestInterestingVerboseReplicationPaths(t *testing.T) {
	paths, prefixes := RequiredPathsFor("repl", true, false)
	if !Interesting("replSetGetStatus.members.0.pingMs", paths, prefixes, true) {
		t.Fatal("expected pingMs to be interesting with verbose replication")
	}
	if Interesting("replSetGetStatus.members.0.pingMs", paths, prefixes, false) {
		t.Fatal("pingMs should not be interesting without verbose replication")
	}
	if !Interesting("serverStatus.metrics.repl.apply.ops", paths, prefixes, true) {
		t.Fatal("expected serverStatus.metrics.repl.apply.ops to be interesting")
	}
	if Interesting("serverStatus.metrics.repl.buffer.count", paths, prefixes, true) {
		t.Fatal("broad metrics.repl path should not be interesting")
	}
	if Interesting("replSetGetStatus.set", paths, prefixes, true) {
		t.Fatal("broad replSetGetStatus path should not be interesting")
	}
}
