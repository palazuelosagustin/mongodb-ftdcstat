package render

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/model"
)

func TestMetadataSummary(t *testing.T) {
	m := model.NewMetadata()
	m.AddDocument(time.Unix(0, 0), "metadata", map[string]any{
		"buildInfo": map[string]any{
			"version":          "8.0.20-8",
			"gitVersion":       "abc",
			"allocator":        "tcmalloc-google",
			"openssl":          map[string]any{"running": "OpenSSL 3.0.13"},
			"perconaFeatures":  []any{"TDE", "LDAP", "TDE"},
			"buildEnvironment": map[string]any{"ccflags": "-O2 lots of flags"},
		},
		"hostInfo": map[string]any{
			"system": map[string]any{"hostname": "db1:27017", "cpuArch": "x86_64", "numCores": 8, "memSizeMB": 32095},
			"extra":  map[string]any{"kernelVersion": "6.8.0"},
		},
		"getCmdLineOpts": map[string]any{
			"argv": []any{"mongod", "-f", "/etc/mongod.conf"},
			"parsed": map[string]any{
				"net":          map[string]any{"port": 27017, "bindIp": "0.0.0.0"},
				"storage":      map[string]any{"wiredTiger": map[string]any{"engineConfig": map[string]any{"cacheSizeGB": 0.5}}},
				"setParameter": map[string]any{"ttlMonitorEnabled": false},
			},
		},
		"getParameter": map[string]any{"maxIndexBuildMemoryUsageMegabytes": 200, "quiet": false},
		"serverStatus": map[string]any{"process": "mongod", "storageEngine": map[string]any{"name": "wiredTiger"}},
	})

	got := MetadataSummary(m)
	// Round-trip through JSON so number types compare as they are emitted.
	var decoded map[string]any
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"process": "mongod",
		"host": map[string]any{
			"hostname": "db1:27017", "arch": "x86_64", "cpuCores": 8.0, "memoryMB": 32095.0, "kernel": "6.8.0",
		},
		"build": map[string]any{
			"version": "8.0.20-8", "gitVersion": "abc", "allocator": "tcmalloc-google", "openssl": "OpenSSL 3.0.13",
			"perconaFeatures": []any{"TDE", "LDAP"}, "storageEngine": "wiredTiger",
		},
		"cmdLineOpts": map[string]any{
			"net.port": 27017.0, "net.bindIp": "0.0.0.0", "storage.wiredTiger.engineConfig.cacheSizeGB": 0.5,
		},
		"parameters": map[string]any{"wtCache": 0.5, "ttlMonitorEnabled": false},
	}
	if !reflect.DeepEqual(decoded, want) {
		gotJSON, _ := json.MarshalIndent(decoded, "", "  ")
		t.Fatalf("summary:\n%s", gotJSON)
	}
}

func TestMetadataSummaryEmpty(t *testing.T) {
	got := MetadataSummary(model.NewMetadata())
	if len(got) != 1 || got["process"] != "mongod" {
		t.Fatalf("summary=%v", got)
	}
}
