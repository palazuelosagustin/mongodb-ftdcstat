package stats

import (
	"math"
	"time"
)

// Column accumulates statistics for one numeric column. Rows where the value
// is missing are counted as nulls.
type Column struct {
	Rows   uint64
	sum    float64
	maxAt  time.Time
	sketch *Sketch
}

func NewColumn() *Column {
	return &Column{sketch: NewSketch()}
}

// Add records one row. ok is false when the column has no value in that row.
func (c *Column) Add(t time.Time, v float64, ok bool) {
	c.Rows++
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if c.sketch.Count() == 0 || v > c.sketch.max {
		c.maxAt = t
	}
	c.sketch.Add(v)
	c.sum += v
}

// Summary is the result for one numeric column. Value fields are nil when the
// column had no values.
type Summary struct {
	N     uint64
	Nulls uint64
	Min   *float64
	Avg   *float64
	P50   *float64
	P95   *float64
	P99   *float64
	Max   *float64
	MaxAt time.Time
}

func (c *Column) Summary() Summary {
	n := c.sketch.Count()
	out := Summary{N: n, Nulls: c.Rows - n}
	if n == 0 {
		return out
	}
	quantile := func(q float64) *float64 {
		v, _ := c.sketch.Quantile(q)
		return &v
	}
	minV, maxV, avg := c.sketch.min, c.sketch.max, c.sum/float64(n)
	out.Min, out.Max, out.Avg = &minV, &maxV, &avg
	out.P50, out.P95, out.P99 = quantile(0.50), quantile(0.95), quantile(0.99)
	out.MaxAt = c.maxAt
	return out
}

// States accumulates a text column such as rsState: how many rows had each
// value and how many times the value changed between consecutive rows.
type States struct {
	Rows    uint64
	Counts  map[string]uint64
	Changes uint64
	order   []string
	last    string
}

func NewStates() *States {
	return &States{Counts: map[string]uint64{}}
}

func (s *States) Add(value string) {
	s.Rows++
	if value == "" {
		return
	}
	if _, seen := s.Counts[value]; !seen {
		s.order = append(s.order, value)
	}
	s.Counts[value]++
	if s.last != "" && value != s.last {
		s.Changes++
	}
	s.last = value
}

// Values returns the values in first-seen order.
func (s *States) Values() []string {
	return append([]string(nil), s.order...)
}
