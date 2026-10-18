package query

import (
	"context"
	"hash/fnv"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	in "github.com/eventrouter/router/internal/metrics/instruments"
)

// MockShape tunes the synthetic series of the mock backend.
type MockShape struct {
	Partitions int
	Branch     string
	MaxBase    float64
	Swing      float64
	Period     time.Duration
}

// DefaultMockShape gives three partitions, one main branch and values below 1000.
var DefaultMockShape = MockShape{Partitions: 3, Branch: "main", MaxBase: 1000, Swing: 0.3, Period: time.Hour}

// mockPatterns read the grouping label and route ids out of a catalog expression.
var (
	mockByPattern    = regexp.MustCompile(`by \((\w+)\)`)
	mockRoutePattern = regexp.MustCompile(string(in.LabelRouteID) + `=~?"([^"]*)"`)
)

// MockQuerier serves deterministic synthetic series seeded by the expression (and so by route id).
type MockQuerier struct {
	now   func() time.Time
	shape MockShape
}

// NewMockQuerier builds the mock backend.
func NewMockQuerier(now func() time.Time) *MockQuerier {
	return &MockQuerier{now: now, shape: DefaultMockShape}
}

// Kind implements Querier.
func (m *MockQuerier) Kind() BackendKind { return BackendMock }

// Instant implements Querier.
func (m *MockQuerier) Instant(_ context.Context, e Expr) (Vector, error) {
	now := m.now()
	sets := m.labelSets(e)
	out := make(Vector, 0, len(sets))
	for _, l := range sets {
		out = append(out, Sample{Labels: l, Value: m.value(e, l, now)})
	}
	return out, nil
}

// Range implements Querier.
func (m *MockQuerier) Range(_ context.Context, e Expr, w Window) ([]Series, error) {
	sets := m.labelSets(e)
	out := make([]Series, 0, len(sets))
	for _, l := range sets {
		s := Series{Labels: l}
		for t := w.Start.Truncate(w.Step); !t.After(w.End); t = t.Add(w.Step) {
			s.Points = append(s.Points, Point{At: t, Value: m.value(e, l, t)})
		}
		out = append(out, s)
	}
	return out, nil
}

// labelSets returns one label set per synthetic series of e.
func (m *MockQuerier) labelSets(e Expr) []Labels {
	by := ""
	for _, match := range mockByPattern.FindAllStringSubmatch(string(e), -1) {
		if match[1] != string(in.LabelLe) {
			by = match[1]
		}
	}
	switch in.MetricLabel(by) {
	case in.LabelRouteID:
		return m.routeSets(e)
	case in.LabelPartition:
		return m.partitionSets()
	case in.LabelBranch:
		return []Labels{{by: m.shape.Branch}}
	}
	return []Labels{{}}
}

func (m *MockQuerier) routeSets(e Expr) []Labels {
	match := mockRoutePattern.FindStringSubmatch(string(e))
	if match == nil {
		return nil
	}
	var out []Labels
	for _, id := range strings.Split(strings.ReplaceAll(match[1], `\`, ""), "|") {
		out = append(out, Labels{string(in.LabelRouteID): id})
	}
	return out
}

func (m *MockQuerier) partitionSets() []Labels {
	out := make([]Labels, m.shape.Partitions)
	for i := range out {
		out[i] = Labels{string(in.LabelPartition): strconv.Itoa(i)}
	}
	return out
}

// value is a smooth wave around a base derived from the expression and labels.
func (m *MockQuerier) value(e Expr, l Labels, t time.Time) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(e))
	for _, k := range []in.MetricLabel{in.LabelRouteID, in.LabelPartition, in.LabelBranch} {
		_, _ = h.Write([]byte(l[string(k)]))
	}
	seed := h.Sum64()
	base := float64(seed%uint64(m.shape.MaxBase)) + 1
	phase := float64(seed%360) * math.Pi / 180
	wave := math.Sin(2*math.Pi*float64(t.Unix())/m.shape.Period.Seconds() + phase)
	return math.Round(base*(1+m.shape.Swing*wave)*100) / 100
}
