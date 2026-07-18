package ftdc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mongodb-ftdcstat/internal/derive"
	"mongodb-ftdcstat/internal/discovery"
	"mongodb-ftdcstat/internal/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SampleReader interface {
	ReadFiles(files []discovery.MetricFile, opts ReaderOptions) (model.Capture, error)
}

type SampleSink func(model.MetricSample) error

type ReaderOptions struct {
	IncludePaths       map[string]bool
	IncludePrefixes    []string
	VerboseReplication bool
	ProcessKind        string
	MaxSamples         int
	TimeRange          model.TimeRange
}

type NativeReader struct{}

func NewNativeReader() NativeReader {
	return NativeReader{}
}

func DefaultReaderOptions() ReaderOptions {
	return ReaderOptionsForKind(model.ProcessKindMongod, "summary", false, false)
}

func ReaderOptionsFor(view string, verbose, pressure bool) ReaderOptions {
	return ReaderOptionsForKind(model.ProcessKindMongod, view, verbose, pressure)
}

func ReaderOptionsForKind(processKind, view string, verbose, pressure bool) ReaderOptions {
	paths, prefixes := derive.RequiredPathsForProcess(processKind, view, verbose, pressure)
	return ReaderOptions{
		IncludePaths:       paths,
		IncludePrefixes:    prefixes,
		VerboseReplication: derive.ViewNeedsVerboseReplication(view, verbose),
		ProcessKind:        processKind,
	}
}

func (r NativeReader) ReadMetadataFiles(files []discovery.MetricFile) (model.Metadata, []model.Warning, error) {
	metadata := model.NewMetadata()
	var warnings []model.Warning
	if len(files) == 0 {
		return metadata, nil, errors.New("no files to read")
	}
	for _, file := range files {
		var fileWarnings []model.Warning
		var err error
		switch file.Kind {
		case discovery.KindJSON:
			fileWarnings, err = r.readJSONFileMetadata(file, &metadata)
		default:
			fileWarnings, err = r.readBinaryFileMetadata(file, &metadata)
		}
		warnings = append(warnings, fileWarnings...)
		if err != nil {
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
		}
	}
	return metadata, warnings, nil
}

func (r NativeReader) ReadFiles(files []discovery.MetricFile, opts ReaderOptions) (model.Capture, error) {
	capture := model.Capture{Metadata: model.NewMetadata()}
	if len(files) == 0 {
		return capture, errors.New("no files to read")
	}
	for _, file := range files {
		capture.Files = append(capture.Files, file.Path)
		var samples []model.MetricSample
		var warnings []model.Warning
		var err error
		switch file.Kind {
		case discovery.KindJSON:
			samples, warnings, err = r.readJSONFile(file, &capture.Metadata, opts)
		default:
			samples, warnings, err = r.readBinaryFile(file, &capture.Metadata, opts)
		}
		capture.Warnings = append(capture.Warnings, warnings...)
		if err != nil {
			capture.Warnings = append(capture.Warnings, model.Warning{Source: file.Path, Message: err.Error()})
			continue
		}
		capture.Samples = append(capture.Samples, samples...)
		if opts.MaxSamples > 0 && len(capture.Samples) >= opts.MaxSamples {
			capture.Samples = capture.Samples[:opts.MaxSamples]
			break
		}
	}
	capture.Samples = derive.MergeSamples(capture.Samples, &capture.Warnings)
	return capture, nil
}

func (r NativeReader) StreamFiles(files []discovery.MetricFile, opts ReaderOptions, sink SampleSink) ([]model.Warning, error) {
	if len(files) == 0 {
		return nil, errors.New("no files to read")
	}
	var warnings []model.Warning
	merger := mergedSampleStream{}
	for _, file := range files {
		var fileWarnings []model.Warning
		var err error
		switch file.Kind {
		case discovery.KindJSON:
			fileWarnings, err = r.streamJSONFile(file, opts, func(sample model.MetricSample) error {
				return merger.Add(sample, sink)
			})
		default:
			fileWarnings, err = r.streamBinaryFile(file, opts, func(sample model.MetricSample) error {
				return merger.Add(sample, sink)
			})
		}
		warnings = append(warnings, fileWarnings...)
		if err != nil {
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
		}
	}
	if err := merger.Flush(sink, &warnings); err != nil {
		return warnings, err
	}
	return warnings, nil
}

func (r NativeReader) readBinaryFile(file discovery.MetricFile, metadata *model.Metadata, opts ReaderOptions) ([]model.MetricSample, []model.Warning, error) {
	f, err := os.Open(file.Path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var samples []model.MetricSample
	var warnings []model.Warning
	reader := bufio.NewReader(f)
	for {
		raw, err := readBSONRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
			break
		}
		chunkSamples, chunkWarnings := r.processRecord(raw, file, metadata, opts)
		warnings = append(warnings, chunkWarnings...)
		samples = append(samples, chunkSamples...)
		if opts.MaxSamples > 0 && len(samples) >= opts.MaxSamples {
			return samples[:opts.MaxSamples], warnings, nil
		}
	}
	return samples, warnings, nil
}

func (r NativeReader) streamBinaryFile(file discovery.MetricFile, opts ReaderOptions, sink SampleSink) ([]model.Warning, error) {
	f, err := os.Open(file.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var warnings []model.Warning
	reader := bufio.NewReader(f)
	for {
		raw, err := readBSONRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
			break
		}
		chunkWarnings, err := r.streamRecord(raw, file, opts, sink)
		warnings = append(warnings, chunkWarnings...)
		if err != nil {
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
		}
	}
	return warnings, nil
}

func readBSONRecord(reader *bufio.Reader) ([]byte, error) {
	header, err := reader.Peek(4)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("truncated BSON length: %w", err)
	}
	length := int(int32(binary.LittleEndian.Uint32(header)))
	if length < 5 {
		return nil, fmt.Errorf("invalid BSON document length %d", length)
	}
	if length > 256*1024*1024 {
		return nil, fmt.Errorf("unreasonable BSON document length %d", length)
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return nil, fmt.Errorf("truncated BSON document: %w", err)
	}
	return raw, nil
}

func (r NativeReader) readJSONFile(file discovery.MetricFile, metadata *model.Metadata, opts ReaderOptions) ([]model.MetricSample, []model.Warning, error) {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, nil, err
	}
	var samples []model.MetricSample
	var warnings []model.Warning
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return samples, warnings, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			var list []json.RawMessage
			if err := json.Unmarshal(raw, &list); err != nil {
				warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
				continue
			}
			for _, item := range list {
				chunkSamples, chunkWarnings := r.processJSONRecord(item, file, metadata, opts)
				warnings = append(warnings, chunkWarnings...)
				samples = append(samples, chunkSamples...)
			}
			continue
		}
		chunkSamples, chunkWarnings := r.processJSONRecord(raw, file, metadata, opts)
		warnings = append(warnings, chunkWarnings...)
		samples = append(samples, chunkSamples...)
	}
	return samples, warnings, nil
}

func (r NativeReader) streamJSONFile(file discovery.MetricFile, opts ReaderOptions, sink SampleSink) ([]model.Warning, error) {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}
	var warnings []model.Warning
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return warnings, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			var list []json.RawMessage
			if err := json.Unmarshal(raw, &list); err != nil {
				warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
				continue
			}
			for _, item := range list {
				chunkWarnings, err := r.streamJSONRecord(item, file, opts, sink)
				warnings = append(warnings, chunkWarnings...)
				if err != nil {
					warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
				}
			}
			continue
		}
		chunkWarnings, err := r.streamJSONRecord(raw, file, opts, sink)
		warnings = append(warnings, chunkWarnings...)
		if err != nil {
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
		}
	}
	return warnings, nil
}

func (r NativeReader) processJSONRecord(raw []byte, file discovery.MetricFile, metadata *model.Metadata, opts ReaderOptions) ([]model.MetricSample, []model.Warning) {
	var doc bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &doc); err != nil {
		return nil, []model.Warning{{Source: file.Path, Message: "unsupported JSON FTDC record: " + err.Error()}}
	}
	rawBSON, err := bson.Marshal(doc)
	if err != nil {
		return nil, []model.Warning{{Source: file.Path, Message: "cannot convert JSON record to BSON: " + err.Error()}}
	}
	return r.processRecord(rawBSON, file, metadata, opts)
}

func (r NativeReader) streamJSONRecord(raw []byte, file discovery.MetricFile, opts ReaderOptions, sink SampleSink) ([]model.Warning, error) {
	var doc bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &doc); err != nil {
		return []model.Warning{{Source: file.Path, Message: "unsupported JSON FTDC record: " + err.Error()}}, nil
	}
	rawBSON, err := bson.Marshal(doc)
	if err != nil {
		return []model.Warning{{Source: file.Path, Message: "cannot convert JSON record to BSON: " + err.Error()}}, nil
	}
	return r.streamRecord(rawBSON, file, opts, sink)
}

func (r NativeReader) processRecord(raw []byte, file discovery.MetricFile, metadata *model.Metadata, opts ReaderOptions) ([]model.MetricSample, []model.Warning) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil, []model.Warning{{Source: file.Path, Message: "cannot decode BSON record: " + err.Error()}}
	}
	docMap := doc.Map()
	ts := recordTime(docMap)
	recordType, _ := numericRecordType(docMap["type"])
	if recordType != 1 {
		metadata.AddDocument(ts, file.Path, doc)
		return nil, nil
	}
	payload, ok := binaryPayload(docMap["data"])
	if !ok {
		return nil, []model.Warning{{Source: file.Path, Message: "metric record has no binary data payload"}}
	}
	samples, refDoc, warnings, err := decodeMetricChunk(payload, file.Path, file.Sequence, opts)
	if refDoc != nil {
		metadata.AddDocument(ts, file.Path, refDoc)
	}
	if err != nil {
		warnings = append(warnings, model.Warning{Source: file.Path, Message: "metric chunk skipped: " + err.Error()})
		return nil, warnings
	}
	return samples, warnings
}

func (r NativeReader) streamRecord(raw []byte, file discovery.MetricFile, opts ReaderOptions, sink SampleSink) ([]model.Warning, error) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return []model.Warning{{Source: file.Path, Message: "cannot decode BSON record: " + err.Error()}}, nil
	}
	docMap := doc.Map()
	recordType, _ := numericRecordType(docMap["type"])
	if recordType != 1 {
		return nil, nil
	}
	payload, ok := binaryPayload(docMap["data"])
	if !ok {
		return []model.Warning{{Source: file.Path, Message: "metric record has no binary data payload"}}, nil
	}
	samples, _, warnings, err := decodeMetricChunk(payload, file.Path, file.Sequence, opts)
	if err != nil {
		warnings = append(warnings, model.Warning{Source: file.Path, Message: "metric chunk skipped: " + err.Error()})
		return warnings, nil
	}
	for _, sample := range samples {
		if err := sink(sample); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (r NativeReader) readBinaryFileMetadata(file discovery.MetricFile, metadata *model.Metadata) ([]model.Warning, error) {
	f, err := os.Open(file.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var warnings []model.Warning
	reader := bufio.NewReader(f)
	for {
		raw, err := readBSONRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
			break
		}
		recordWarnings := processRecordMetadata(raw, file, metadata)
		warnings = append(warnings, recordWarnings...)
	}
	return warnings, nil
}

func (r NativeReader) readJSONFileMetadata(file discovery.MetricFile, metadata *model.Metadata) ([]model.Warning, error) {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}
	var warnings []model.Warning
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return warnings, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			var list []json.RawMessage
			if err := json.Unmarshal(raw, &list); err != nil {
				warnings = append(warnings, model.Warning{Source: file.Path, Message: err.Error()})
				continue
			}
			for _, item := range list {
				recordWarnings := processJSONRecordMetadata(item, file, metadata)
				warnings = append(warnings, recordWarnings...)
			}
			continue
		}
		recordWarnings := processJSONRecordMetadata(raw, file, metadata)
		warnings = append(warnings, recordWarnings...)
	}
	return warnings, nil
}

func numericRecordType(v any) (int, bool) {
	switch value := v.(type) {
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case int:
		return value, true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func binaryPayload(v any) ([]byte, bool) {
	switch value := v.(type) {
	case primitive.Binary:
		return value.Data, true
	case []byte:
		return value, true
	default:
		return nil, false
	}
}

func recordTime(doc map[string]any) time.Time {
	for _, key := range []string{"_id", "timestamp", "time"} {
		if v, ok := doc[key]; ok {
			if t, ok := model.AsTime(model.ToPlain(v)); ok {
				return t
			}
		}
	}
	if v, ok := doc["doc"]; ok {
		if m, ok := model.ToPlain(v).(map[string]any); ok {
			for _, path := range []string{"end", "start"} {
				if value, ok := model.Lookup(m, path); ok {
					if t, ok := model.AsTime(value); ok {
						return t
					}
				}
			}
		}
	}
	return time.Time{}
}

func processJSONRecordMetadata(raw []byte, file discovery.MetricFile, metadata *model.Metadata) []model.Warning {
	var doc bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &doc); err != nil {
		return []model.Warning{{Source: file.Path, Message: "unsupported JSON FTDC record: " + err.Error()}}
	}
	rawBSON, err := bson.Marshal(doc)
	if err != nil {
		return []model.Warning{{Source: file.Path, Message: "cannot convert JSON record to BSON: " + err.Error()}}
	}
	return processRecordMetadata(rawBSON, file, metadata)
}

func processRecordMetadata(raw []byte, file discovery.MetricFile, metadata *model.Metadata) []model.Warning {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return []model.Warning{{Source: file.Path, Message: "cannot decode BSON record: " + err.Error()}}
	}
	docMap := doc.Map()
	ts := recordTime(docMap)
	recordType, _ := numericRecordType(docMap["type"])
	if recordType != 1 {
		metadata.AddDocument(ts, file.Path, doc)
		return nil
	}
	payload, ok := binaryPayload(docMap["data"])
	if !ok {
		return []model.Warning{{Source: file.Path, Message: "metric record has no binary data payload"}}
	}
	refDoc, warnings, err := decodeMetricChunkMetadata(payload, file.Path)
	if refDoc != nil {
		metadata.AddDocument(ts, file.Path, refDoc)
	}
	if err != nil {
		warnings = append(warnings, model.Warning{Source: file.Path, Message: "metric chunk skipped: " + err.Error()})
	}
	return warnings
}

type flatMetric struct {
	Path  string
	Value int64
}

var (
	chunkBlockPool = sync.Pool{
		New: func() any {
			return make([]byte, 0, 64*1024)
		},
	}
	flatMetricsPool = sync.Pool{
		New: func() any {
			return make([]flatMetric, 0, 256)
		},
	}
)

func readCompressedChunkBlock(payload []byte) (block []byte, expectedSize int, release func(), err error) {
	if len(payload) < 5 {
		return nil, 0, nil, errors.New("payload shorter than FTDC header")
	}
	expectedSize = int(binary.LittleEndian.Uint32(payload[:4]))
	zr, err := zlib.NewReader(bytes.NewReader(payload[4:]))
	if err != nil {
		return nil, expectedSize, nil, err
	}
	defer zr.Close()

	buf := chunkBlockPool.Get().([]byte)
	buf = buf[:0]
	if expectedSize > 0 {
		if cap(buf) < expectedSize {
			chunkBlockPool.Put(buf)
			buf = make([]byte, expectedSize)
		} else {
			buf = buf[:expectedSize]
		}
		if _, err := io.ReadFull(zr, buf); err != nil {
			chunkBlockPool.Put(buf[:0])
			return nil, expectedSize, nil, err
		}
	} else {
		var readErr error
		buf, readErr = readAllGrow(buf, zr)
		if readErr != nil {
			chunkBlockPool.Put(buf[:0])
			return nil, expectedSize, nil, readErr
		}
	}
	release = func() {
		chunkBlockPool.Put(buf[:0])
	}
	return buf, expectedSize, release, nil
}

func readAllGrow(buf []byte, r io.Reader) ([]byte, error) {
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return buf, err
		}
	}
}

func borrowedFlatMetrics(refDoc bson.D) ([]flatMetric, func()) {
	buf := flatMetricsPool.Get().([]flatMetric)
	buf = appendFlattenMetrics(buf[:0], refDoc, "")
	return buf, func() {
		flatMetricsPool.Put(buf[:0])
	}
}

func setSampleValue(fields []map[string]float64, index int, path string, value float64, capHint int) {
	if fields[index] == nil {
		fields[index] = make(map[string]float64, capHint)
	}
	fields[index][path] = value
}

type mergedSampleStream struct {
	pending model.MetricSample
	have    bool
}

func (m *mergedSampleStream) Add(sample model.MetricSample, sink SampleSink) error {
	if sample.Time.IsZero() {
		return nil
	}
	if !m.have {
		m.pending = sample
		m.have = true
		return nil
	}
	if sample.Time.Equal(m.pending.Time) {
		m.pending = sample
		return nil
	}
	if sample.Time.Before(m.pending.Time) {
		return fmt.Errorf("samples out of order: %s before %s", sample.Time.Format(time.RFC3339), m.pending.Time.Format(time.RFC3339))
	}
	if err := sink(m.pending); err != nil {
		return err
	}
	m.pending = sample
	return nil
}

func (m *mergedSampleStream) Flush(sink SampleSink, warnings *[]model.Warning) error {
	if !m.have {
		return nil
	}
	if err := sink(m.pending); err != nil {
		return err
	}
	m.have = false
	return nil
}

func decodeMetricChunk(payload []byte, source string, sourceIndex int, opts ReaderOptions) ([]model.MetricSample, any, []model.Warning, error) {
	var warnings []model.Warning
	block, expectedSize, releaseBlock, err := readCompressedChunkBlock(payload)
	if err != nil {
		return nil, nil, warnings, err
	}
	defer releaseBlock()
	if expectedSize > 0 && expectedSize != len(block) {
		warnings = append(warnings, model.Warning{Source: source, Message: fmt.Sprintf("chunk uncompressed size header %d differs from actual %d", expectedSize, len(block))})
	}
	if len(block) < 12 {
		return nil, nil, warnings, errors.New("decompressed block too short")
	}
	docLen := int(int32(binary.LittleEndian.Uint32(block[:4])))
	if docLen < 5 || docLen > len(block) {
		return nil, nil, warnings, fmt.Errorf("invalid reference BSON length %d", docLen)
	}
	refRaw := block[:docLen]
	var refDoc bson.D
	if err := bson.Unmarshal(refRaw, &refDoc); err != nil {
		return nil, nil, warnings, fmt.Errorf("cannot decode reference BSON: %w", err)
	}
	offset := docLen
	if offset+8 > len(block) {
		return nil, refDoc, warnings, errors.New("chunk missing metric and sample counts")
	}
	a := binary.LittleEndian.Uint32(block[offset : offset+4])
	b := binary.LittleEndian.Uint32(block[offset+4 : offset+8])
	offset += 8

	metrics, releaseMetrics := borrowedFlatMetrics(refDoc)
	defer releaseMetrics()
	metricCount, deltaCount := a, b
	if int(a) != len(metrics) && int(b) == len(metrics) {
		metricCount, deltaCount = b, a
		warnings = append(warnings, model.Warning{Source: source, Message: "chunk count order appears reversed; decoded using fallback order"})
	}
	if int(metricCount) != len(metrics) {
		warnings = append(warnings, model.Warning{Source: source, Message: fmt.Sprintf("metric count mismatch: header=%d flattened=%d", metricCount, len(metrics))})
	}

	keepCount := 0
	for _, metric := range metrics {
		path := canonicalMetricPath(metric.Path, opts.ProcessKind)
		if derive.Interesting(path, opts.IncludePaths, opts.IncludePrefixes, opts.VerboseReplication) {
			keepCount++
		}
	}

	sampleCount := int(deltaCount) + 1
	sampleFields := make([]map[string]float64, sampleCount)
	reader := bytes.NewReader(block[offset:])
	zeroRun := int64(0)
	for _, metric := range metrics {
		path := canonicalMetricPath(metric.Path, opts.ProcessKind)
		keep := derive.Interesting(path, opts.IncludePaths, opts.IncludePrefixes, opts.VerboseReplication)
		current := metric.Value
		if keep {
			setSampleValue(sampleFields, 0, path, float64(current), keepCount)
		}
		for i := uint32(0); i < deltaCount; i++ {
			var delta int64
			if zeroRun > 0 {
				zeroRun--
			} else {
				var err error
				delta, err = readSignedVarint(reader)
				if err != nil {
					return nil, refDoc, warnings, err
				}
				if delta == 0 {
					zeroRun, err = readSignedVarint(reader)
					if err != nil {
						return nil, refDoc, warnings, err
					}
				}
			}
			current += delta
			if keep {
				setSampleValue(sampleFields, int(i)+1, path, float64(current), keepCount)
			}
		}
	}
	samples := make([]model.MetricSample, 0, sampleCount)
	for i := 0; i < sampleCount; i++ {
		fields := sampleFields[i]
		if fields == nil {
			continue
		}
		ts := sampleTimestamp(fields)
		if ts.IsZero() {
			continue
		}
		if !opts.TimeRange.IsZero() && !opts.TimeRange.Contains(ts) {
			continue
		}
		samples = append(samples, model.MetricSample{
			Time:        ts,
			Source:      source,
			SourceIndex: sourceIndex,
			Values:      fields,
		})
	}
	return samples, refDoc, warnings, nil
}

func decodeMetricChunkMetadata(payload []byte, source string) (any, []model.Warning, error) {
	var warnings []model.Warning
	block, expectedSize, releaseBlock, err := readCompressedChunkBlock(payload)
	if err != nil {
		return nil, warnings, err
	}
	defer releaseBlock()
	if expectedSize > 0 && expectedSize != len(block) {
		warnings = append(warnings, model.Warning{Source: source, Message: fmt.Sprintf("chunk uncompressed size header %d differs from actual %d", expectedSize, len(block))})
	}
	if len(block) < 12 {
		return nil, warnings, errors.New("decompressed block too short")
	}
	docLen := int(int32(binary.LittleEndian.Uint32(block[:4])))
	if docLen < 5 || docLen > len(block) {
		return nil, warnings, fmt.Errorf("invalid reference BSON length %d", docLen)
	}
	refRaw := block[:docLen]
	var refDoc bson.D
	if err := bson.Unmarshal(refRaw, &refDoc); err != nil {
		return nil, warnings, fmt.Errorf("cannot decode reference BSON: %w", err)
	}
	return refDoc, warnings, nil
}

func sampleTimestamp(fields map[string]float64) time.Time {
	for _, path := range []string{"start", "end", "serverStatus.localTime"} {
		if v, ok := fields[path]; ok && v > 0 {
			return time.UnixMilli(int64(v)).UTC()
		}
	}
	return time.Time{}
}

func flattenMetrics(value any, prefix string) []flatMetric {
	return appendFlattenMetrics(nil, value, prefix)
}

func appendFlattenMetrics(out []flatMetric, value any, prefix string) []flatMetric {
	switch v := value.(type) {
	case bson.D:
		for _, elem := range v {
			path := elem.Key
			if prefix != "" {
				path = prefix + "." + elem.Key
			}
			out = appendFlattenMetrics(out, elem.Value, path)
		}
	case bson.A:
		for i, elem := range v {
			path := strconvPath(prefix, i)
			out = appendFlattenMetrics(out, elem, path)
		}
	case bool:
		if v {
			out = append(out, flatMetric{Path: prefix, Value: 1})
		} else {
			out = append(out, flatMetric{Path: prefix, Value: 0})
		}
	case int:
		out = append(out, flatMetric{Path: prefix, Value: int64(v)})
	case int32:
		out = append(out, flatMetric{Path: prefix, Value: int64(v)})
	case int64:
		out = append(out, flatMetric{Path: prefix, Value: v})
	case float64:
		out = append(out, flatMetric{Path: prefix, Value: int64(v)})
	case primitive.DateTime:
		out = append(out, flatMetric{Path: prefix, Value: int64(v)})
	case time.Time:
		out = append(out, flatMetric{Path: prefix, Value: v.UnixMilli()})
	case primitive.Timestamp:
		out = append(out, flatMetric{Path: prefix + ".t", Value: int64(v.T)})
		out = append(out, flatMetric{Path: prefix + ".i", Value: int64(v.I)})
	}
	return out
}

func strconvPath(prefix string, i int) string {
	if prefix == "" {
		return fmt.Sprint(i)
	}
	return prefix + "." + fmt.Sprint(i)
}

func readSignedVarint(reader *bytes.Reader) (int64, error) {
	var res uint64
	var shift uint
	for i := 0; ; i++ {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		res |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return int64(res), nil
		}
		shift += 7
		if i > 9 {
			return 0, errors.New("varint overflow")
		}
	}
}

func canonicalMetricPath(path, processKind string) string {
	_ = processKind
	switch {
	case path == "common.start":
		return "start"
	case path == "common.end":
		return "end"
	case strings.HasPrefix(path, "common.serverStatus."):
		return strings.TrimPrefix(path, "common.")
	case strings.HasPrefix(path, "common.systemMetrics."):
		return strings.TrimPrefix(path, "common.")
	case strings.HasPrefix(path, "common.transportLayerStats."):
		return strings.TrimPrefix(path, "common.")
	default:
		return path
	}
}

func SourceName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func LooksLikeInterim(path string) bool {
	name := filepath.Base(path)
	return strings.Contains(name, "interim")
}
