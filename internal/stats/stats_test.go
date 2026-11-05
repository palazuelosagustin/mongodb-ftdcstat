package stats

import (
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"
)

func TestSketchQuantilesWithinRelativeAccuracy(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	s := NewSketch()
	var values []float64
	for i := 0; i < 200000; i++ {
		v := math.Exp(rng.NormFloat64() * 3)
		if i%10 == 0 {
			v = 0
		}
		values = append(values, v)
		s.Add(v)
	}
	sort.Float64s(values)
	for _, q := range []float64{0, 0.05, 0.5, 0.95, 0.99, 1} {
		got, ok := s.Quantile(q)
		if !ok {
			t.Fatalf("q=%v: empty", q)
		}
		want := values[int(q*float64(len(values)-1))]
		if want == 0 {
			if got != 0 {
				t.Fatalf("q=%v got %v want 0", q, got)
			}
			continue
		}
		if rel := math.Abs(got-want) / want; rel > RelativeAccuracy+1e-9 {
			t.Fatalf("q=%v got %v want %v (rel err %v)", q, got, want, rel)
		}
	}
}

func TestSketchNegativeValues(t *testing.T) {
	s := NewSketch()
	for _, v := range []float64{-100, -10, 0, 10, 100} {
		s.Add(v)
	}
	if got, _ := s.Quantile(0.25); math.Abs(got+10)/10 > RelativeAccuracy {
		t.Fatalf("p25=%v want ~-10", got)
	}
	if got, _ := s.Quantile(0.5); got != 0 {
		t.Fatalf("p50=%v want 0", got)
	}
	if got, _ := s.Quantile(0); got != -100 {
		t.Fatalf("min=%v want -100", got)
	}
}

func TestSketchEmpty(t *testing.T) {
	if _, ok := NewSketch().Quantile(0.5); ok {
		t.Fatal("empty sketch returned a quantile")
	}
}

func TestColumnSummary(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	c := NewColumn()
	c.Add(base, 1, true)
	c.Add(base.Add(time.Second), 5, true)
	c.Add(base.Add(2*time.Second), 0, false)
	c.Add(base.Add(3*time.Second), 3, true)
	s := c.Summary()
	if s.N != 3 || s.Nulls != 1 {
		t.Fatalf("n=%d nulls=%d", s.N, s.Nulls)
	}
	if *s.Min != 1 || *s.Max != 5 || *s.Avg != 3 {
		t.Fatalf("min=%v max=%v avg=%v", *s.Min, *s.Max, *s.Avg)
	}
	if !s.MaxAt.Equal(base.Add(time.Second)) {
		t.Fatalf("maxAt=%v", s.MaxAt)
	}
}

func TestColumnSummaryAllNull(t *testing.T) {
	c := NewColumn()
	c.Add(time.Unix(0, 0), 0, false)
	s := c.Summary()
	if s.N != 0 || s.Nulls != 1 || s.Min != nil || s.P95 != nil {
		t.Fatalf("summary=%+v", s)
	}
}

func TestStates(t *testing.T) {
	s := NewStates()
	for _, v := range []string{"PRIMARY", "PRIMARY", "SECONDARY", "", "PRIMARY"} {
		s.Add(v)
	}
	if s.Rows != 5 || s.Counts["PRIMARY"] != 3 || s.Counts["SECONDARY"] != 1 || s.Changes != 2 {
		t.Fatalf("states=%+v", s)
	}
	if got := s.Values(); len(got) != 2 || got[0] != "PRIMARY" {
		t.Fatalf("values=%v", got)
	}
}
