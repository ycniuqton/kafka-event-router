package preview

import (
	"context"

	"github.com/eventrouter/router/pkg/kafka"
)

// SampleSource loads sample records for one SampleMode within a budget.
type SampleSource interface {
	Mode() SampleMode
	Load(ctx context.Context, req LoadRequest, budget *kafka.SamplingBudget) (LoadResult, error)
}

// SampleSourceManager selects the source registered for a mode; business logic uses only this.
type SampleSourceManager struct {
	sources map[SampleMode]SampleSource
}

// NewSampleSourceManager registers the given sources by their modes.
func NewSampleSourceManager(sources ...SampleSource) *SampleSourceManager {
	m := &SampleSourceManager{sources: map[SampleMode]SampleSource{}}
	for _, s := range sources {
		m.sources[s.Mode()] = s
	}
	return m
}

// For returns the source of mode, or a problem pointing at /mode.
func (m *SampleSourceManager) For(mode SampleMode) (SampleSource, error) {
	s, ok := m.sources[mode]
	if !ok {
		return nil, ErrUnknownMode.At(string(FieldMode))
	}
	return s, nil
}

// Modes lists the registered modes.
func (m *SampleSourceManager) Modes() []SampleMode {
	out := make([]SampleMode, 0, len(m.sources))
	for mode := range m.sources {
		out = append(out, mode)
	}
	return out
}
