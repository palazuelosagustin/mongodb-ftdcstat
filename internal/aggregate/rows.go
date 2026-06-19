package aggregate

import (
	"fmt"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
)

type RowBucketAverager struct {
	bucket  time.Duration
	current bucketState
}

type bucketState struct {
	start   time.Time
	fields  map[string]fieldState
	marker  string
	process string
}

type fieldState struct {
	sum         float64
	count       int
	text        string
	haveNumber  bool
	haveText    bool
	mixedText   bool
}

func NewRowBucketAverager(bucket time.Duration) *RowBucketAverager {
	return &RowBucketAverager{bucket: bucket}
}

func AverageRows(rows []derive.Row, bucket time.Duration) []derive.Row {
	if bucket <= 0 || len(rows) == 0 {
		return append([]derive.Row(nil), rows...)
	}
	averager := NewRowBucketAverager(bucket)
	out := make([]derive.Row, 0, len(rows))
	for _, row := range rows {
		flushed := averager.Add(row)
		out = append(out, flushed...)
	}
	return append(out, averager.Flush()...)
}

func (a *RowBucketAverager) Add(row derive.Row) []derive.Row {
	if a.bucket <= 0 {
		return []derive.Row{row}
	}
	start := bucketStart(row.Time, a.bucket)
	if a.current.start.IsZero() {
		a.reset(start)
	} else if !start.Equal(a.current.start) {
		flushed := a.Flush()
		a.reset(start)
		a.addRow(row)
		return flushed
	}
	a.addRow(row)
	return nil
}

func (a *RowBucketAverager) Flush() []derive.Row {
	if a.current.start.IsZero() {
		return nil
	}
	values := make(map[string]any, len(a.current.fields))
	for key, field := range a.current.fields {
		switch {
		case field.haveNumber && field.count > 0:
			values[key] = field.sum / float64(field.count)
		case field.haveText:
			if field.mixedText {
				values[key] = "MIXED"
			} else {
				values[key] = field.text
			}
		}
	}
	row := derive.Row{
		Time:          a.current.start,
		Marker:        a.current.marker,
		ProcessMarker: a.current.process,
		Values:        values,
	}
	a.current = bucketState{}
	return []derive.Row{row}
}

func bucketStart(ts time.Time, bucket time.Duration) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	return ts.UTC().Truncate(bucket)
}

func (a *RowBucketAverager) reset(start time.Time) {
	a.current = bucketState{
		start:  start,
		fields: map[string]fieldState{},
	}
}

func (a *RowBucketAverager) addRow(row derive.Row) {
	if a.current.marker == "" && row.Marker != "" {
		a.current.marker = row.Marker
	}
	if a.current.process == "" && row.ProcessMarker != "" {
		a.current.process = row.ProcessMarker
	}
	for key, value := range row.Values {
		field := a.current.fields[key]
		if number, ok := model.AsFloat(value); ok {
			field.sum += number
			field.count++
			field.haveNumber = true
			a.current.fields[key] = field
			continue
		}
		text := stringify(value)
		if text == "" {
			continue
		}
		if !field.haveText {
			field.text = text
			field.haveText = true
		} else if field.text != text {
			field.mixedText = true
		}
		a.current.fields[key] = field
	}
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		if v == "" {
			return ""
		}
		return v
	default:
		return fmt.Sprint(v)
	}
}
