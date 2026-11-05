// Package stats holds streaming summary statistics for derived metric rows.
package stats

import (
	"math"
	"sort"
)

// RelativeAccuracy is the maximum relative error of Sketch quantiles.
const RelativeAccuracy = 0.01

// Sketch estimates quantiles of a stream of values in bounded memory.
//
// Values are counted in logarithmic buckets, so any quantile is returned
// within RelativeAccuracy of a value that is actually at that rank. Zero is
// counted exactly. The number of buckets grows with the log of the value
// range, not with the number of values.
type Sketch struct {
	gamma    float64
	logGamma float64
	pos      map[int]uint64
	neg      map[int]uint64
	zeros    uint64
	count    uint64
	min, max float64
}

func NewSketch() *Sketch {
	gamma := (1 + RelativeAccuracy) / (1 - RelativeAccuracy)
	return &Sketch{
		gamma:    gamma,
		logGamma: math.Log(gamma),
		pos:      map[int]uint64{},
		neg:      map[int]uint64{},
	}
}

// minIndexable avoids huge negative bucket indexes for denormal values; such
// values are treated as zero.
const minIndexable = 1e-12

func (s *Sketch) Add(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if s.count == 0 || v < s.min {
		s.min = v
	}
	if s.count == 0 || v > s.max {
		s.max = v
	}
	s.count++
	switch {
	case v > minIndexable:
		s.pos[s.index(v)]++
	case v < -minIndexable:
		s.neg[s.index(-v)]++
	default:
		s.zeros++
	}
}

func (s *Sketch) Count() uint64 {
	return s.count
}

// Quantile returns the estimated q-quantile (0 <= q <= 1). It returns false
// when the sketch is empty.
func (s *Sketch) Quantile(q float64) (float64, bool) {
	if s.count == 0 {
		return 0, false
	}
	if q <= 0 {
		return s.min, true
	}
	if q >= 1 {
		return s.max, true
	}
	rank := uint64(q * float64(s.count-1))
	var seen uint64

	negKeys := sortedKeys(s.neg)
	for i := len(negKeys) - 1; i >= 0; i-- {
		seen += s.neg[negKeys[i]]
		if seen > rank {
			return s.clamp(-s.value(negKeys[i])), true
		}
	}
	seen += s.zeros
	if seen > rank {
		return s.clamp(0), true
	}
	for _, k := range sortedKeys(s.pos) {
		seen += s.pos[k]
		if seen > rank {
			return s.clamp(s.value(k)), true
		}
	}
	return s.max, true
}

func (s *Sketch) index(v float64) int {
	return int(math.Ceil(math.Log(v) / s.logGamma))
}

// value is the representative of bucket k: every value in the bucket is
// within RelativeAccuracy of it.
func (s *Sketch) value(k int) float64 {
	return 2 * math.Pow(s.gamma, float64(k)) / (s.gamma + 1)
}

func (s *Sketch) clamp(v float64) float64 {
	return math.Min(math.Max(v, s.min), s.max)
}

func sortedKeys(m map[int]uint64) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
