package model

import (
	"testing"
	"time"
)

func TestServerStatusCaptureStoresScalarsOnly(t *testing.T) {
	m := NewMetadata()
	ts := time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC)
	m.AddDocument(ts, "chunk", map[string]any{
		"serverStatus": map[string]any{
			"connections": map[string]any{
				"current":   9,
				"available": 400,
			},
			"storageEngine": map[string]any{"name": "wiredTiger"},
			"repl":          map[string]any{"setName": "rs0"},
		},
	})
	if got := m.NetworkMaxConnDisplay(); got != "409" {
		t.Fatalf("maxConn=%q", got)
	}
	if got := m.StorageEngineName(); got != "wiredTiger" {
		t.Fatalf("storageEngine=%q", got)
	}
	if _, ok := m.Latest["serverStatus"]; ok {
		t.Fatal("serverStatus latest doc should not be retained")
	}
	if len(m.History["serverStatus"]) != 0 {
		t.Fatal("serverStatus history should not be retained")
	}
	doc, ok := m.LatestDoc("serverStatus")
	if !ok {
		t.Fatal("expected compact serverStatus doc")
	}
	if got, _ := Lookup(doc, "storageEngine.name"); got != "wiredTiger" {
		t.Fatalf("compact doc=%#v", doc)
	}
}

func TestReplSnapshotCapturedWithoutStatusHistory(t *testing.T) {
	m := NewMetadata()
	ts := time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC)
	m.AddDocument(ts, "config", map[string]any{
		"replSetGetConfig": map[string]any{
			"config": map[string]any{
				"_id": "rs0",
				"members": []any{
					map[string]any{"host": "h1:27017"},
					map[string]any{"host": "h2:27017"},
				},
			},
		},
	})
	m.AddDocument(ts, "status", map[string]any{
		"replSetGetStatus": map[string]any{
			"set": "rs0",
			"members": []any{
				map[string]any{"name": "h1:27017"},
				map[string]any{"name": "h2:27017"},
			},
		},
	})
	set, members := m.ReplSetSnapshot()
	if set != "rs0" || len(members) != 2 {
		t.Fatalf("snapshot set=%q members=%#v", set, members)
	}
	if len(m.History["replSetGetStatus"]) != 0 {
		t.Fatal("replSetGetStatus history should not be retained")
	}
	if _, ok := m.Latest["replSetGetConfig"]; !ok {
		t.Fatal("expected latest replSetGetConfig")
	}
}

func TestServerStatusReplMembersFillSnapshot(t *testing.T) {
	m := NewMetadata()
	m.AddDocument(time.Unix(0, 0), "chunk", map[string]any{
		"serverStatus": map[string]any{
			"process": "mongod",
			"repl": map[string]any{
				"setName":  "shard01",
				"hosts":    []any{"h1:27017", "h2:27017"},
				"passives": []any{"h3:27017"},
				"arbiters": []any{"h4:27017"},
			},
		},
	})
	set, members := m.ReplSetSnapshot()
	if set != "shard01" {
		t.Fatalf("set=%q", set)
	}
	want := []ReplMemberState{
		{Label: "node1", Name: "h1:27017"},
		{Label: "node2", Name: "h2:27017"},
		{Label: "node3", Name: "h3:27017"},
		{Label: "node4", Name: "h4:27017"},
	}
	if len(members) != len(want) {
		t.Fatalf("members=%#v", members)
	}
	for i := range want {
		if members[i] != want[i] {
			t.Fatalf("members[%d]=%#v want %#v", i, members[i], want[i])
		}
	}
}

func TestReplConfigOverridesServerStatusReplMembers(t *testing.T) {
	m := NewMetadata()
	m.AddDocument(time.Unix(0, 0), "serverStatus", map[string]any{
		"serverStatus": map[string]any{
			"repl": map[string]any{
				"setName": "rs0",
				"hosts":   []any{"fallback2:27017", "fallback1:27017"},
			},
		},
	})
	m.AddDocument(time.Unix(1, 0), "config", map[string]any{
		"replSetGetConfig": map[string]any{
			"config": map[string]any{
				"_id": "rs0",
				"members": []any{
					map[string]any{"host": "config1:27017"},
					map[string]any{"host": "config2:27017"},
				},
			},
		},
	})
	m.AddDocument(time.Unix(2, 0), "new-status", map[string]any{
		"serverStatus": map[string]any{
			"repl": map[string]any{
				"hosts": []any{"extra:27017"},
			},
		},
	})
	_, members := m.ReplSetSnapshot()
	want := []ReplMemberState{
		{Label: "node1", Name: "config1:27017"},
		{Label: "node2", Name: "config2:27017"},
	}
	if len(members) != len(want) {
		t.Fatalf("members=%#v", members)
	}
	for i := range want {
		if members[i] != want[i] {
			t.Fatalf("members[%d]=%#v want %#v", i, members[i], want[i])
		}
	}
}

func TestSummaryUsesCompactServerStatus(t *testing.T) {
	m := NewMetadata()
	m.AddDocument(time.Unix(0, 0), "chunk", map[string]any{
		"serverStatus": map[string]any{
			"connections":   map[string]any{"current": 1, "available": 2},
			"storageEngine": map[string]any{"name": "wiredTiger"},
		},
	})
	summary := m.Summary()
	status, ok := summary["serverStatus"].(map[string]any)
	if !ok {
		t.Fatalf("summary=%#v", summary)
	}
	if _, ok := status["connections"]; ok {
		t.Fatalf("compact summary should not retain full connections map: %#v", status)
	}
}

func TestProcessKindComesOnlyFromServerStatusProcess(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
		want string
	}{
		{
			name: "router only stays unknown",
			doc: map[string]any{
				"router": map[string]any{
					"connPoolStats": map[string]any{"totalInUse": 3},
				},
			},
			want: ProcessKindUnknown,
		},
		{
			name: "serverStatus without process stays unknown",
			doc: map[string]any{
				"serverStatus": map[string]any{
					"storageEngine": map[string]any{"name": "wiredTiger"},
				},
				"router": map[string]any{
					"connPoolStats": map[string]any{"totalInUse": 3},
				},
			},
			want: ProcessKindUnknown,
		},
		{
			name: "mongod process wins over router metrics",
			doc: map[string]any{
				"serverStatus": map[string]any{"process": "mongod"},
				"router": map[string]any{
					"connPoolStats": map[string]any{"totalInUse": 3},
				},
			},
			want: ProcessKindMongod,
		},
		{
			name: "mongos process selects mongos",
			doc: map[string]any{
				"serverStatus": map[string]any{"process": "mongos"},
			},
			want: ProcessKindMongos,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMetadata()
			m.AddDocument(time.Unix(0, 0), "chunk", tt.doc)
			if got := m.ProcessKind(); got != tt.want {
				t.Fatalf("processKind=%q want %q", got, tt.want)
			}
		})
	}
}

func TestCommonRootDetectsMongosAndUnwrapsServerStatus(t *testing.T) {
	m := NewMetadata()
	m.AddDocument(time.Unix(0, 0), "chunk", map[string]any{
		"common": map[string]any{
			"serverStatus": map[string]any{
				"process": "mongos",
				"connections": map[string]any{
					"current":   32,
					"available": 65504,
				},
			},
		},
		"router": map[string]any{
			"connPoolStats": map[string]any{
				"totalInUse": 3,
			},
		},
	})
	if got := m.ProcessKind(); got != ProcessKindMongos {
		t.Fatalf("processKind=%q", got)
	}
	if got := m.NetworkMaxConnDisplay(); got != "65536" {
		t.Fatalf("maxConn=%q", got)
	}
}
