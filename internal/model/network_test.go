package model

import (
	"testing"
	"time"
)

func TestDeriveNetworkMaxConnFromServerStatus(t *testing.T) {
	got, ok := DeriveNetworkMaxConn(map[string]any{
		"connections": map[string]any{
			"current":   12,
			"available": 65524,
		},
	})
	if !ok || got != "65536" {
		t.Fatalf("DeriveNetworkMaxConn=%q ok=%v", got, ok)
	}
}

func TestDeriveNetworkMaxConnMissingFields(t *testing.T) {
	if _, ok := DeriveNetworkMaxConn(map[string]any{
		"connections": map[string]any{"current": 12},
	}); ok {
		t.Fatal("expected missing available to fail")
	}
	if _, ok := DeriveNetworkMaxConn(map[string]any{}); ok {
		t.Fatal("expected empty doc to fail")
	}
}

func TestMetadataStoresFirstUsableNetworkMaxConn(t *testing.T) {
	m := NewMetadata()
	late := time.Date(2026, 6, 4, 19, 0, 0, 0, time.UTC)
	early := time.Date(2026, 6, 4, 18, 0, 0, 0, time.UTC)
	m.AddDocument(late, "late", map[string]any{
		"serverStatus": map[string]any{
			"connections": map[string]any{"current": 9, "available": 400},
		},
	})
	if got := m.NetworkMaxConnDisplay(); got != "409" {
		t.Fatalf("first usable maxConn=%q", got)
	}
	m.AddDocument(early, "early", map[string]any{
		"serverStatus": map[string]any{
			"connections": map[string]any{"current": 12, "available": 65524},
		},
	})
	if got := m.NetworkMaxConnDisplay(); got != "65536" {
		t.Fatalf("earlier usable maxConn=%q", got)
	}
}

func TestMetadataNetworkMaxConnDefaultsToDash(t *testing.T) {
	m := NewMetadata()
	if got := m.NetworkMaxConnDisplay(); got != "-" {
		t.Fatalf("maxConn=%q", got)
	}
}
