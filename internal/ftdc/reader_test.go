package ftdc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"mongodb-ftdcstat/internal/discovery"
	"mongodb-ftdcstat/internal/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestDecodeMetricChunkReconstructsSamples(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	ref := bson.D{
		{Key: "start", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "serverStatus", Value: bson.D{
			{Key: "opcounters", Value: bson.D{{Key: "insert", Value: int64(10)}}},
		}},
	}
	payload := buildChunkPayload(t, ref, map[string][]int64{
		"start":                          {1000, 1000},
		"serverStatus.opcounters.insert": {5, 7},
	})
	samples, refDoc, warnings, err := decodeMetricChunk(payload, "synthetic", 0, DefaultReaderOptions())
	if err != nil {
		t.Fatal(err)
	}
	if refDoc == nil {
		t.Fatal("missing reference doc")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if len(samples) != 3 {
		t.Fatalf("got %d samples", len(samples))
	}
	if got := samples[2].Values["serverStatus.opcounters.insert"]; got != 22 {
		t.Fatalf("insert value=%v", got)
	}
	if !samples[2].Time.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("time=%s", samples[2].Time)
	}
}

func TestFlattenMetricsParsesBSONReferenceDocument(t *testing.T) {
	ref := bson.D{
		{Key: "start", Value: primitive.DateTime(1000)},
		{Key: "nested", Value: bson.D{{Key: "flag", Value: true}}},
		{Key: "members", Value: bson.A{bson.D{{Key: "optimeDate", Value: primitive.DateTime(2000)}}}},
	}
	metrics := flattenMetrics(ref, "")
	got := map[string]int64{}
	for _, metric := range metrics {
		got[metric.Path] = metric.Value
	}
	if got["start"] != 1000 {
		t.Fatalf("start=%d", got["start"])
	}
	if got["nested.flag"] != 1 {
		t.Fatalf("flag=%d", got["nested.flag"])
	}
	if got["members.0.optimeDate"] != 2000 {
		t.Fatalf("array date=%d", got["members.0.optimeDate"])
	}
}

func TestNativeReaderHandlesTruncatedInterimFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.interim")
	start := time.Unix(100, 0).UTC()
	payload := buildChunkPayload(t, bson.D{
		{Key: "start", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "serverStatus", Value: bson.D{{Key: "opcounters", Value: bson.D{{Key: "insert", Value: int64(1)}}}}},
	}, map[string][]int64{
		"start":                          {1000},
		"serverStatus.opcounters.insert": {1},
	})
	record, err := bson.Marshal(bson.D{
		{Key: "_id", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: primitive.Binary{Subtype: 0, Data: payload}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data := append(record, []byte{99, 0, 0, 0}...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	reader := NewNativeReader()
	capture, err := reader.ReadFiles([]discovery.MetricFile{{Path: path, Kind: discovery.KindInterim}}, DefaultReaderOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Samples) != 2 {
		t.Fatalf("got %d samples", len(capture.Samples))
	}
	if len(capture.Warnings) == 0 {
		t.Fatal("expected warning for truncated interim")
	}
}

func TestNativeReaderReadsExportedJSONRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")
	start := time.Unix(100, 0).UTC()
	payload := buildChunkPayload(t, bson.D{
		{Key: "start", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "serverStatus", Value: bson.D{{Key: "opcounters", Value: bson.D{{Key: "insert", Value: int64(1)}}}}},
	}, map[string][]int64{
		"start":                          {1000},
		"serverStatus.opcounters.insert": {1},
	})
	record := bson.D{
		{Key: "_id", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: primitive.Binary{Subtype: 0, Data: payload}},
	}
	jsonRecord, err := bson.MarshalExtJSON(record, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(jsonRecord, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := NewNativeReader()
	capture, err := reader.ReadFiles([]discovery.MetricFile{{Path: path, Kind: discovery.KindJSON}}, DefaultReaderOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Samples) != 2 {
		t.Fatalf("got %d samples", len(capture.Samples))
	}
}

func TestNativeReaderFiltersSamplesByTimeRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.2026-01-01T00-00-00Z-00000")
	start := time.Unix(100, 0).UTC()
	payload := buildChunkPayload(t, bson.D{
		{Key: "start", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "serverStatus", Value: bson.D{{Key: "opcounters", Value: bson.D{{Key: "insert", Value: int64(1)}}}}},
	}, map[string][]int64{
		"start":                          {1000, 1000},
		"serverStatus.opcounters.insert": {1, 1},
	})
	record, err := bson.Marshal(bson.D{
		{Key: "_id", Value: primitive.NewDateTimeFromTime(start)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: primitive.Binary{Subtype: 0, Data: payload}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := DefaultReaderOptions()
	opts.TimeRange.From = start.Add(time.Second)
	opts.TimeRange.To = start.Add(2 * time.Second)
	capture, err := NewNativeReader().ReadFiles([]discovery.MetricFile{{Path: path, Kind: discovery.KindMetrics}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Samples) != 1 {
		t.Fatalf("got %d samples", len(capture.Samples))
	}
	if !capture.Samples[0].Time.Equal(start.Add(time.Second)) {
		t.Fatalf("got sample time %s", capture.Samples[0].Time)
	}
}

func TestSampleDiagnosticDataSmoke(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, warnings, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no sample files discovered")
	}
	reader := NewNativeReader()
	opts := DefaultReaderOptions()
	opts.MaxSamples = 25
	capture, err := reader.ReadFiles(files, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected discovery warnings: %#v", warnings)
	}
	if len(capture.Samples) == 0 {
		t.Fatal("no samples decoded from diagnostic.data.27000")
	}
	if _, ok := capture.Metadata.LatestDoc("buildInfo"); !ok {
		t.Fatal("expected buildInfo metadata from diagnostic.data.27000")
	}
}

func TestStreamFilesMatchesReadFiles(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}
	reader := NewNativeReader()
	opts := DefaultReaderOptions()
	capture, err := reader.ReadFiles(files, opts)
	if err != nil {
		t.Fatal(err)
	}
	metadata, warnings, err := reader.ReadMetadataFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected metadata warnings: %#v", warnings)
	}
	var got []model.MetricSample
	streamWarnings, err := reader.StreamFiles(files, opts, func(sample model.MetricSample) error {
		got = append(got, sample)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamWarnings) != len(capture.Warnings) {
		t.Fatalf("stream warnings=%d capture warnings=%d", len(streamWarnings), len(capture.Warnings))
	}
	if !reflect.DeepEqual(got, capture.Samples) {
		t.Fatalf("streamed samples mismatch read files")
	}
	if !reflect.DeepEqual(metadata.Summary(), capture.Metadata.Summary()) {
		t.Fatalf("stream metadata summary mismatch")
	}
}

func TestCanonicalMetricPathUnwrapsCommonForAllProcessKinds(t *testing.T) {
	for _, processKind := range []string{model.ProcessKindUnknown, model.ProcessKindMongod, model.ProcessKindMongos} {
		t.Run(processKind, func(t *testing.T) {
			cases := map[string]string{
				"common.start": "start",
				"common.end":   "end",
				"common.serverStatus.connections.current":     "serverStatus.connections.current",
				"common.systemMetrics.cpu.user_ms":            "systemMetrics.cpu.user_ms",
				"common.transportLayerStats.ingress.sessions": "transportLayerStats.ingress.sessions",
				"router.connPoolStats.totalInUse":             "router.connPoolStats.totalInUse",
			}
			for path, want := range cases {
				if got := canonicalMetricPath(path, processKind); got != want {
					t.Fatalf("canonicalMetricPath(%q, %q)=%q want %q", path, processKind, got, want)
				}
			}
		})
	}
}

func TestMongosDiagnosticDataCanonicalizesCommonMetrics(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "mongos.diagnostic.data")
	if _, err := os.Stat(root); err != nil {
		t.Skip("mongos.diagnostic.data sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	reader := NewNativeReader()
	metadata, _, err := reader.ReadMetadataFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ProcessKind() != model.ProcessKindMongos {
		t.Fatalf("processKind=%q", metadata.ProcessKind())
	}
	opts := ReaderOptions{IncludePrefixes: []string{""}, MaxSamples: 3, ProcessKind: metadata.ProcessKind()}
	capture, err := reader.ReadFiles(files[:1], opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Samples) == 0 {
		t.Fatal("no samples decoded from mongos diagnostic data")
	}
	sample := capture.Samples[0]
	if _, ok := sample.Values["serverStatus.connections.current"]; !ok {
		t.Fatalf("missing canonical serverStatus path in sample: %#v", sample.Values)
	}
	for key := range sample.Values {
		if len(key) >= len("common.") && key[:len("common.")] == "common." {
			t.Fatalf("unexpected raw common.* metric path after canonicalization: %q", key)
		}
	}
	if _, ok := sample.Values["router.connPoolStats.totalInUse"]; !ok {
		t.Fatalf("missing router metric in sample: %#v", sample.Values)
	}
}

func buildChunkPayload(t testing.TB, ref bson.D, deltas map[string][]int64) []byte {
	t.Helper()
	refRaw, err := bson.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	metrics := flattenMetrics(ref, "")
	if len(metrics) == 0 {
		t.Fatal("no metrics in reference doc")
	}
	deltaCount := len(deltas[metrics[0].Path])
	var block bytes.Buffer
	block.Write(refRaw)
	if err := binary.Write(&block, binary.LittleEndian, uint32(len(metrics))); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(&block, binary.LittleEndian, uint32(deltaCount)); err != nil {
		t.Fatal(err)
	}
	for _, metric := range metrics {
		list := deltas[metric.Path]
		if len(list) != deltaCount {
			t.Fatalf("deltas for %s have len %d want %d", metric.Path, len(list), deltaCount)
		}
		for _, delta := range list {
			writeVarint(&block, uint64(delta))
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(block.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var payload bytes.Buffer
	if err := binary.Write(&payload, binary.LittleEndian, uint32(block.Len())); err != nil {
		t.Fatal(err)
	}
	payload.Write(compressed.Bytes())
	return payload.Bytes()
}

func writeVarint(buf *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		buf.WriteByte(byte(value) | 0x80)
		value >>= 7
	}
	buf.WriteByte(byte(value))
}

func TestReadMetadataDerivesNetworkMaxConn(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 1 {
		files = files[:1]
	}
	metadata, _, err := NewNativeReader().ReadMetadataFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if got := metadata.NetworkMaxConnDisplay(); got == "-" || got == "" {
		t.Fatalf("expected derived maxConn from metadata, got %q", got)
	}
	if len(metadata.History["serverStatus"]) != 0 {
		t.Fatalf("serverStatus history=%d", len(metadata.History["serverStatus"]))
	}
	if len(metadata.History["replSetGetStatus"]) != 0 {
		t.Fatalf("replSetGetStatus history=%d", len(metadata.History["replSetGetStatus"]))
	}
	if _, ok := metadata.Latest["serverStatus"]; ok {
		t.Fatal("serverStatus latest doc should not be retained")
	}
}

func TestReadMetadataHeapStaysBoundedWithoutServerStatusHistory(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "diagnostic.data.27000")
	if _, err := os.Stat(root); err != nil {
		t.Skip("diagnostic.data.27000 sample directory not present")
	}
	files, _, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		files = files[:3]
	}
	before := readMetadataHeapAlloc(t)
	metadata, _, err := NewNativeReader().ReadMetadataFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	after := readMetadataHeapAlloc(t)
	if metadata.NetworkMaxConnDisplay() == "-" {
		t.Fatal("expected derived maxConn")
	}
	if delta := int64(after) - int64(before); delta > 400*1024*1024 {
		t.Fatalf("metadata heap delta=%.1fMB", float64(delta)/1e6)
	}
}

func readMetadataHeapAlloc(t *testing.T) uint64 {
	t.Helper()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

var _ = model.Warning{}
