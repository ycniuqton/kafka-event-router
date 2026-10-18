package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/platform/logging"
)

// IntentType is what the UI does when the row's action is clicked (matches web ActionIntent).
type IntentType string

const (
	IntentRouteDetail    IntentType = "routeDetail"
	IntentRouteEdit      IntentType = "routeEdit"
	IntentTestConnection IntentType = "testConnection"
	IntentTogglePause    IntentType = "togglePause"
)

// IntentTarget names route-detail tabs and builder steps intents open.
type IntentTarget string

const (
	TabDeadLetters IntentTarget = "dead"
	StepFilter     IntentTarget = "filter"
)

// ActionLabel is the button text of an attention row.
type ActionLabel string

const (
	LabelOpen  ActionLabel = "Open"
	LabelPause ActionLabel = "Pause"
	LabelTest  ActionLabel = "Test"
	LabelRetry ActionLabel = "Retry"
)

// ActionIntent is a serialisable action the UI resolves to navigation or a store action.
type ActionIntent struct {
	Type         IntentType   `json:"type"`
	RouteID      string       `json:"routeId,omitempty"`
	ConnectionID string       `json:"connectionId,omitempty"`
	Tab          IntentTarget `json:"tab,omitempty"`
	Step         IntentTarget `json:"step,omitempty"`
}

// Action is the one button of an attention row.
type Action struct {
	Label  ActionLabel  `json:"label"`
	Intent ActionIntent `json:"intent"`
}

// AttentionItem is one "Needs attention" row.
type AttentionItem struct {
	ID     string                `json:"id"`
	Kind   metrics.AttentionKind `json:"kind"`
	Tone   metrics.Tone          `json:"tone"`
	Title  string                `json:"title"`
	Detail string                `json:"detail"`
	Action Action                `json:"action"`
}

// ItemID is the dedupe key of a row: kind plus subject.
type ItemID struct{}

// Of returns "<kind>:<subject>".
func (ItemID) Of(kind metrics.AttentionKind, subject string) string {
	return fmt.Sprintf("%s:%s", kind, subject)
}

// Snapshot is the shared input of one detection pass: live routes and thresholds.
type Snapshot struct {
	Routes     []RouteRow
	Thresholds metrics.Thresholds
}

// Names indexes the live routes by id.
func (s Snapshot) Names() map[string]RouteRow {
	out := make(map[string]RouteRow, len(s.Routes))
	for _, r := range s.Routes {
		out[string(r.ID)] = r
	}
	return out
}

// AttentionDetector is the base of every needs-attention rule.
type AttentionDetector interface {
	Kind() metrics.AttentionKind
	Detect(ctx context.Context, snap Snapshot) ([]AttentionItem, error)
}

// ErrUnknownDetector reports a configured detector kind that is not registered.
type ErrUnknownDetector struct {
	Kind metrics.AttentionKind
}

func (e ErrUnknownDetector) Error() string {
	return "dashboard: unknown attention detector " + string(e.Kind)
}

// AttentionManager runs the enabled detectors in parallel, omits failing ones, dedupes and sorts.
type AttentionManager struct {
	routes     RouteSnapshotReader
	detectors  []AttentionDetector
	thresholds metrics.Thresholds
	log        *slog.Logger
}

// NewAttentionManager enables the configured kinds out of the registered detectors.
func NewAttentionManager(routes RouteSnapshotReader, t metrics.Thresholds, log *slog.Logger, enabled []metrics.AttentionKind, registered ...AttentionDetector) (*AttentionManager, error) {
	byKind := map[metrics.AttentionKind]AttentionDetector{}
	for _, d := range registered {
		byKind[d.Kind()] = d
	}
	m := &AttentionManager{routes: routes, thresholds: t, log: log}
	for _, k := range enabled {
		d, ok := byKind[k]
		if !ok {
			return nil, ErrUnknownDetector{Kind: k}
		}
		m.detectors = append(m.detectors, d)
	}
	return m, nil
}

// Detect returns the merged rows, most severe first.
func (m *AttentionManager) Detect(ctx context.Context) ([]AttentionItem, error) {
	rows, err := m.routes.LiveRoutes(ctx)
	if err != nil {
		return nil, err
	}
	snap := Snapshot{Routes: rows, Thresholds: m.thresholds}
	results := make([][]AttentionItem, len(m.detectors))
	var wg sync.WaitGroup
	for i, d := range m.detectors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = m.run(ctx, d, snap)
		}()
	}
	wg.Wait()
	return AttentionMerger{}.Merge(results), nil
}

// run executes one detector; a failure is logged and yields no rows.
func (m *AttentionManager) run(ctx context.Context, d AttentionDetector, snap Snapshot) []AttentionItem {
	items, err := d.Detect(ctx, snap)
	if err != nil {
		m.log.Warn("dashboard: attention detector failed", string(logging.AttrError), err, string(AttrDetector), string(d.Kind()))
		return nil
	}
	return items
}

// AttentionMerger dedupes rows by id and orders err before warn, then by title.
type AttentionMerger struct{}

// Merge flattens detector results.
func (AttentionMerger) Merge(results [][]AttentionItem) []AttentionItem {
	seen := map[string]bool{}
	out := []AttentionItem{}
	for _, items := range results {
		for _, it := range items {
			if !seen[it.ID] {
				seen[it.ID] = true
				out = append(out, it)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := metrics.ToneRank[out[i].Tone], metrics.ToneRank[out[j].Tone]
		if ri != rj {
			return ri > rj
		}
		return out[i].Title < out[j].Title
	})
	return out
}
