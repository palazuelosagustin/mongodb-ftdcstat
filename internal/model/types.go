package model

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Warning struct {
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

func (w Warning) String() string {
	if w.Source == "" {
		return w.Message
	}
	return w.Source + ": " + w.Message
}

type MetricSample struct {
	Time        time.Time          `json:"time"`
	Source      string             `json:"source,omitempty"`
	SourceIndex int                `json:"-"`
	Values      map[string]float64 `json:"values,omitempty"`
}

type TimeRange struct {
	From time.Time
	To   time.Time
}

func (r TimeRange) IsZero() bool {
	return r.From.IsZero() && r.To.IsZero()
}

func (r TimeRange) Contains(t time.Time) bool {
	if t.IsZero() {
		return false
	}
	if !r.From.IsZero() && t.Before(r.From) {
		return false
	}
	if !r.To.IsZero() && !t.Before(r.To) {
		return false
	}
	return true
}

func (r TimeRange) Overlaps(start, end time.Time) bool {
	if r.IsZero() || start.IsZero() || end.IsZero() || !end.After(start) {
		return true
	}
	if !r.To.IsZero() && !start.Before(r.To) {
		return false
	}
	if !r.From.IsZero() && !end.After(r.From) {
		return false
	}
	return true
}

func (s MetricSample) Get(path string) (float64, bool) {
	v, ok := s.Values[path]
	return v, ok
}

func (s MetricSample) GetAny(paths ...string) (float64, bool) {
	for _, path := range paths {
		if v, ok := s.Get(path); ok {
			return v, true
		}
	}
	return 0, false
}

type Capture struct {
	Samples  []MetricSample `json:"samples,omitempty"`
	Metadata Metadata       `json:"metadata"`
	Warnings []Warning      `json:"warnings,omitempty"`
	Files    []string       `json:"files,omitempty"`
}

type MetadataRecord struct {
	Name      string         `json:"name"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
	Source    string         `json:"source,omitempty"`
	Doc       map[string]any `json:"doc,omitempty"`
}

type Metadata struct {
	Latest   map[string]MetadataRecord   `json:"latest,omitempty"`
	History  map[string][]MetadataRecord `json:"-"`
	Warnings []Warning                   `json:"warnings,omitempty"`
}

func NewMetadata() Metadata {
	return Metadata{Latest: map[string]MetadataRecord{}, History: map[string][]MetadataRecord{}}
}

func (m *Metadata) AddDocument(ts time.Time, source string, doc any) {
	if m.Latest == nil {
		m.Latest = map[string]MetadataRecord{}
	}
	if m.History == nil {
		m.History = map[string][]MetadataRecord{}
	}
	converted := ToPlain(doc)
	root, ok := converted.(map[string]any)
	if !ok {
		return
	}
	if inner, ok := root["doc"]; ok {
		if innerMap, ok := ToPlain(inner).(map[string]any); ok {
			root = innerMap
		}
	}
	for name, value := range root {
		child, ok := ToPlain(value).(map[string]any)
		if !ok {
			continue
		}
		m.addRecord(MetadataRecord{
			Name:      name,
			Timestamp: bestTimestamp(ts, child),
			Source:    source,
			Doc:       child,
		})
	}
}

func (m *Metadata) addRecord(record MetadataRecord) {
	if trackMetadataHistory(record.Name) {
		m.History[record.Name] = append(m.History[record.Name], record)
	}
	old, exists := m.Latest[record.Name]
	if exists && !record.Timestamp.IsZero() && !old.Timestamp.IsZero() && record.Timestamp.Before(old.Timestamp) {
		return
	}
	m.Latest[record.Name] = record
}

func trackMetadataHistory(name string) bool {
	return name == "replSetGetConfig" || name == "replSetGetStatus" || name == "serverStatus"
}

func bestTimestamp(fallback time.Time, doc map[string]any) time.Time {
	for _, path := range []string{"end", "start", "localTime", "system.currentTime"} {
		if v, ok := Lookup(doc, path); ok {
			if t, ok := AsTime(v); ok {
				return t
			}
		}
	}
	return fallback
}

func (m Metadata) LatestDoc(name string) (map[string]any, bool) {
	if m.Latest == nil {
		return nil, false
	}
	record, ok := m.Latest[name]
	return record.Doc, ok
}

func (m Metadata) LatestRecord(name string) (MetadataRecord, bool) {
	if m.Latest == nil {
		return MetadataRecord{}, false
	}
	record, ok := m.Latest[name]
	return record, ok
}

func (m Metadata) Records(name string) []MetadataRecord {
	if m.History == nil {
		return nil
	}
	records := append([]MetadataRecord(nil), m.History[name]...)
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i].Timestamp
		right := records[j].Timestamp
		if left.Equal(right) {
			return records[i].Source < records[j].Source
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})
	return records
}

func (m Metadata) Summary() map[string]any {
	out := map[string]any{}
	for key, record := range m.Latest {
		switch key {
		case "buildInfo", "hostInfo", "getCmdLineOpts", "getParameter", "replSetGetConfig", "replSetGetStatus", "serverStatus":
			out[key] = record.Doc
		}
	}
	return out
}

func ToPlain(v any) any {
	switch value := v.(type) {
	case nil:
		return nil
	case bson.D:
		m := map[string]any{}
		for _, elem := range value {
			m[elem.Key] = ToPlain(elem.Value)
		}
		return m
	case bson.M:
		m := map[string]any{}
		for key, elem := range value {
			m[key] = ToPlain(elem)
		}
		return m
	case bson.A:
		a := make([]any, len(value))
		for i, elem := range value {
			a[i] = ToPlain(elem)
		}
		return a
	case []any:
		a := make([]any, len(value))
		for i, elem := range value {
			a[i] = ToPlain(elem)
		}
		return a
	case primitive.DateTime:
		return utcTimeFromDateTime(value)
	case primitive.Timestamp:
		return map[string]any{"t": value.T, "i": value.I}
	case primitive.Binary:
		return nil
	default:
		return value
	}
}

func Lookup(doc map[string]any, path string) (any, bool) {
	if doc == nil || path == "" {
		return nil, false
	}
	var cur any = doc
	for _, part := range strings.Split(path, ".") {
		switch value := cur.(type) {
		case map[string]any:
			next, ok := value[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(value) {
				return nil, false
			}
			cur = value[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func AsString(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	case bool:
		return strconv.FormatBool(value), true
	case fmt.Stringer:
		return value.String(), true
	default:
		if value, ok := AsFloat(v); ok {
			if math.Trunc(value) == value {
				return strconv.FormatInt(int64(value), 10), true
			}
			return strconv.FormatFloat(value, 'f', -1, 64), true
		}
		return "", false
	}
}

func AsFloat(v any) (float64, bool) {
	switch value := v.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case primitive.DateTime:
		return float64(int64(value)), true
	case time.Time:
		return float64(value.UnixMilli()), true
	default:
		return 0, false
	}
}

func AsTime(v any) (time.Time, bool) {
	switch value := v.(type) {
	case time.Time:
		return value.UTC(), true
	case primitive.DateTime:
		return utcTimeFromDateTime(value), true
	default:
		if number, ok := AsFloat(v); ok {
			if number > 1e12 {
				return time.UnixMilli(int64(number)).UTC(), true
			}
			if number > 1e9 {
				return time.Unix(int64(number), 0).UTC(), true
			}
		}
		return time.Time{}, false
	}
}

func utcTimeFromDateTime(value primitive.DateTime) time.Time {
	return time.UnixMilli(int64(value)).UTC()
}
