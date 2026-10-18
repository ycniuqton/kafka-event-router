// Package latency records end-to-end latencies in a log-linear histogram (≤1.6% relative error) that can be merged
// and shipped as sparse buckets between processes.
package latency

import (
	"math"
	"math/bits"
	"time"
)

// Layout is the bucket geometry of the histogram: values are microseconds, each power of two holds HalfSub buckets.
type Layout struct {
	SubBits uint
	Sub     int64
	HalfSub int64
	Buckets int
}

// Geometry is the single layout every histogram uses, so sparse buckets from any process merge exactly.
var Geometry = Layout{SubBits: 7, Sub: 128, HalfSub: 64, Buckets: 128 + 57*64}

// Units are the unit conversions the histogram reports in.
var Units = struct {
	Micro   time.Duration
	MsPerUs float64
}{Micro: time.Microsecond, MsPerUs: 0.001}

// Histogram counts latencies; the zero value is not usable, use New.
type Histogram struct {
	counts []uint64
	total  uint64
	sumUs  float64
	minUs  int64
	maxUs  int64
	skewed uint64
}

// Summary is the JSON form of a histogram's percentiles in milliseconds.
type Summary struct {
	Count  uint64  `json:"count"`
	MinMs  float64 `json:"minMs"`
	MeanMs float64 `json:"meanMs"`
	P50Ms  float64 `json:"p50Ms"`
	P95Ms  float64 `json:"p95Ms"`
	P99Ms  float64 `json:"p99Ms"`
	P999Ms float64 `json:"p999Ms"`
	MaxMs  float64 `json:"maxMs"`
	Skewed uint64  `json:"negativeClockSkew"`
}

// Sparse is the non-zero buckets of a histogram (bucket index → count) plus its exact extremes.
type Sparse struct {
	Buckets map[int]uint64 `json:"b"`
	MinUs   int64          `json:"minUs"`
	MaxUs   int64          `json:"maxUs"`
	SumUs   float64        `json:"sumUs"`
	Skewed  uint64         `json:"skewed,omitempty"`
}

// New returns an empty histogram.
func New() *Histogram {
	return &Histogram{counts: make([]uint64, Geometry.Buckets), minUs: math.MaxInt64}
}

// Record adds one latency; negative values (sender clock ahead) count as zero and as skewed.
func (h *Histogram) Record(d time.Duration) {
	us := int64(d / Units.Micro)
	if us < 0 {
		h.skewed++
		us = 0
	}
	h.counts[Geometry.index(us)]++
	h.total++
	h.sumUs += float64(us)
	h.minUs = min(h.minUs, us)
	h.maxUs = max(h.maxUs, us)
}

// Count is the number of recorded values.
func (h *Histogram) Count() uint64 {
	return h.total
}

// Merge adds every value of other into h.
func (h *Histogram) Merge(other *Histogram) {
	for i, c := range other.counts {
		h.counts[i] += c
	}
	h.absorb(other.total, other.sumUs, other.minUs, other.maxUs, other.skewed)
}

// MergeSparse adds a shipped sparse histogram into h.
func (h *Histogram) MergeSparse(s Sparse) {
	var total uint64
	for i, c := range s.Buckets {
		if i >= 0 && i < len(h.counts) {
			h.counts[i] += c
			total += c
		}
	}
	h.absorb(total, s.SumUs, s.MinUs, s.MaxUs, s.Skewed)
}

// Sparse returns the non-zero buckets for shipping.
func (h *Histogram) Sparse() Sparse {
	out := Sparse{Buckets: map[int]uint64{}, MinUs: h.minUs, MaxUs: h.maxUs, SumUs: h.sumUs, Skewed: h.skewed}
	for i, c := range h.counts {
		if c > 0 {
			out.Buckets[i] = c
		}
	}
	return out
}

// Quantile returns the value at quantile q (0..1); the upper bound of its bucket, capped at the exact maximum.
func (h *Histogram) Quantile(q float64) time.Duration {
	if h.total == 0 {
		return 0
	}
	rank := uint64(math.Ceil(q * float64(h.total)))
	rank = max(rank, 1)
	var seen uint64
	for i, c := range h.counts {
		seen += c
		if seen >= rank {
			return time.Duration(min(Geometry.upper(i), h.maxUs)) * Units.Micro
		}
	}
	return time.Duration(h.maxUs) * Units.Micro
}

// Summary returns the percentiles in milliseconds.
func (h *Histogram) Summary() Summary {
	if h.total == 0 {
		return Summary{}
	}
	return Summary{
		Count: h.total, MinMs: float64(h.minUs) * Units.MsPerUs, MeanMs: h.sumUs / float64(h.total) * Units.MsPerUs,
		P50Ms: h.quantileMs(0.50), P95Ms: h.quantileMs(0.95), P99Ms: h.quantileMs(0.99), P999Ms: h.quantileMs(0.999),
		MaxMs: float64(h.maxUs) * Units.MsPerUs, Skewed: h.skewed,
	}
}

func (h *Histogram) quantileMs(q float64) float64 {
	return float64(h.Quantile(q)/Units.Micro) * Units.MsPerUs
}

func (h *Histogram) absorb(total uint64, sumUs float64, minUs, maxUs int64, skewed uint64) {
	if total == 0 {
		return
	}
	h.total += total
	h.sumUs += sumUs
	h.minUs = min(h.minUs, minUs)
	h.maxUs = max(h.maxUs, maxUs)
	h.skewed += skewed
}

func (l Layout) index(us int64) int {
	if us < l.Sub {
		return int(us)
	}
	e := uint(bits.Len64(uint64(us))) - l.SubBits
	m := us >> e
	return int(l.Sub + int64(e-1)*l.HalfSub + (m - l.HalfSub))
}

func (l Layout) upper(i int) int64 {
	if int64(i) < l.Sub {
		return int64(i)
	}
	rest := int64(i) - l.Sub
	e := uint(rest/l.HalfSub) + 1
	m := rest%l.HalfSub + l.HalfSub
	return ((m + 1) << e) - 1
}
