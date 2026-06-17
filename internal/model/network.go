package model

import (
	"math"
	"strconv"
)

func (m Metadata) NetworkMaxConnDisplay() string {
	if !m.haveNetworkMaxConn {
		return "-"
	}
	return m.networkMaxConn
}

func (m *Metadata) maybeSetNetworkMaxConn(record MetadataRecord) {
	if record.Name != "serverStatus" {
		return
	}
	value, ok := DeriveNetworkMaxConn(record.Doc)
	if !ok {
		return
	}
	ts := record.Timestamp
	if !m.haveNetworkMaxConn {
		m.networkMaxConn = value
		m.networkMaxConnTime = ts
		m.haveNetworkMaxConn = true
		return
	}
	if ts.IsZero() {
		return
	}
	if m.networkMaxConnTime.IsZero() || ts.Before(m.networkMaxConnTime) {
		m.networkMaxConn = value
		m.networkMaxConnTime = ts
	}
}

func DeriveNetworkMaxConn(doc map[string]any) (string, bool) {
	current, ok := connectionCount(doc, "current")
	if !ok {
		return "", false
	}
	available, ok := connectionCount(doc, "available")
	if !ok {
		return "", false
	}
	return formatWholeNumber(current + available), true
}

func connectionCount(doc map[string]any, field string) (float64, bool) {
	if v, ok := Lookup(doc, "connections."+field); ok {
		return AsFloat(v)
	}
	if v, ok := Lookup(doc, "serverStatus.connections."+field); ok {
		return AsFloat(v)
	}
	return 0, false
}

func formatWholeNumber(value float64) string {
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
