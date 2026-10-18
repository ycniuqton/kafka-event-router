package dashboard

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

// LogAttr names log attributes of the dashboard.
type LogAttr string

const AttrDetector LogAttr = "detector"

// AttentionText holds the user-facing wording of attention rows.
type AttentionText string

const (
	TitleRouteError     AttentionText = "%s is in error"
	DetailRouteError    AttentionText = "The runner stopped processing. Open the route to see failed events."
	TitleConnection     AttentionText = "%s is %s"
	DetailConnection    AttentionText = "Health check failing · %d route(s) affected"
	TitleLagGrowing     AttentionText = "%s lag is growing"
	DetailLagGrowing    AttentionText = "Consumer lag %.0f and rising. Pause the route or add replicas."
	TitleZeroMatch      AttentionText = "%s matched 0 events"
	DetailZeroMatch     AttentionText = "Read %.0f events in the last %s but the filter passed none. Check the filter."
	DetailRouteErrorMsg AttentionText = "Runner error: %s"
)

// RouteErrorDetector lists routes whose runtime reports an error.
type RouteErrorDetector struct{}

func (RouteErrorDetector) Kind() metrics.AttentionKind { return metrics.AttentionRouteError }

// Detect implements AttentionDetector.
func (d RouteErrorDetector) Detect(_ context.Context, snap Snapshot) ([]AttentionItem, error) {
	out := []AttentionItem{}
	for _, r := range snap.Routes {
		if r.Status != routes.StatusError {
			continue
		}
		detail := string(DetailRouteError)
		if r.ErrorMessage != "" {
			detail = fmt.Sprintf(string(DetailRouteErrorMsg), r.ErrorMessage)
		}
		out = append(out, AttentionItem{ID: ItemID{}.Of(d.Kind(), string(r.ID)), Kind: d.Kind(), Tone: metrics.ToneErr,
			Title: fmt.Sprintf(string(TitleRouteError), r.Name), Detail: detail,
			Action: Action{Label: LabelOpen, Intent: ActionIntent{Type: IntentRouteDetail, RouteID: string(r.ID), Tab: TabDeadLetters}}})
	}
	return out, nil
}

// ConnectionHealthDetector lists connections failing their health check.
type ConnectionHealthDetector struct {
	Connections ConnectionHealthReader
}

func (ConnectionHealthDetector) Kind() metrics.AttentionKind {
	return metrics.AttentionConnectionHealth
}

// Detect implements AttentionDetector.
func (d ConnectionHealthDetector) Detect(ctx context.Context, _ Snapshot) ([]AttentionItem, error) {
	rows, err := d.Connections.Unhealthy(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AttentionItem, 0, len(rows))
	for _, c := range rows {
		out = append(out, AttentionItem{ID: ItemID{}.Of(d.Kind(), c.ID), Kind: d.Kind(), Tone: metrics.ToneErr,
			Title:  fmt.Sprintf(string(TitleConnection), c.Name, c.Health),
			Detail: fmt.Sprintf(string(DetailConnection), c.AffectedRoutes),
			Action: Action{Label: LabelTest, Intent: ActionIntent{Type: IntentTestConnection, ConnectionID: c.ID}}})
	}
	return out, nil
}

// LagGrowthDetector lists running routes whose lag is above the warn threshold and still rising.
type LagGrowthDetector struct {
	Querier query.Querier
}

func (LagGrowthDetector) Kind() metrics.AttentionKind { return metrics.AttentionLagGrowing }

// Detect implements AttentionDetector.
func (d LagGrowthDetector) Detect(ctx context.Context, snap Snapshot) ([]AttentionItem, error) {
	cat := query.ExprCatalog{}
	var lag, deriv query.Vector
	err := query.NewBatch(d.Querier).
		Instant(cat.Build(query.QueryLagByRoute, query.Params{}), &lag).
		Instant(cat.Build(query.QueryLagDerivByRoute, query.Params{Range: snap.Thresholds.LagGrowthWindow}), &deriv).Run(ctx)
	if err != nil {
		return nil, err
	}
	label := string(instruments.LabelRouteID)
	lags, slopes, names := lag.ByLabel(label), deriv.ByLabel(label), snap.Names()
	out := []AttentionItem{}
	for id, value := range lags {
		r, ok := names[id]
		if !ok || r.Status != routes.StatusRunning || slopes[id] <= 0 || value <= float64(snap.Thresholds.RouteLagWarn) {
			continue
		}
		out = append(out, AttentionItem{ID: ItemID{}.Of(d.Kind(), id), Kind: d.Kind(), Tone: snap.Thresholds.RouteTone(value),
			Title: fmt.Sprintf(string(TitleLagGrowing), r.Name), Detail: fmt.Sprintf(string(DetailLagGrowing), value),
			Action: Action{Label: LabelPause, Intent: ActionIntent{Type: IntentTogglePause, RouteID: id}}})
	}
	return out, nil
}

// ZeroMatchDetector lists running routes that read events but whose filter passed none in the window.
type ZeroMatchDetector struct {
	Querier query.Querier
}

func (ZeroMatchDetector) Kind() metrics.AttentionKind { return metrics.AttentionZeroMatch }

// Detect implements AttentionDetector.
func (d ZeroMatchDetector) Detect(ctx context.Context, snap Snapshot) ([]AttentionItem, error) {
	cat, window := query.ExprCatalog{}, snap.Thresholds.ZeroMatchWindow
	var consumed, filtered query.Vector
	err := query.NewBatch(d.Querier).
		Instant(cat.Build(query.QueryConsumedByRoute, query.Params{Range: window}), &consumed).
		Instant(cat.Build(query.QueryFilteredByRoute, query.Params{Range: window}), &filtered).Run(ctx)
	if err != nil {
		return nil, err
	}
	label := string(instruments.LabelRouteID)
	read, dropped, names := consumed.ByLabel(label), filtered.ByLabel(label), snap.Names()
	out := []AttentionItem{}
	for id, n := range read {
		r, ok := names[id]
		if !ok || r.Status != routes.StatusRunning || n <= 0 || n-dropped[id] > 0 {
			continue
		}
		out = append(out, AttentionItem{ID: ItemID{}.Of(d.Kind(), id), Kind: d.Kind(), Tone: metrics.ToneWarn,
			Title:  fmt.Sprintf(string(TitleZeroMatch), r.Name),
			Detail: fmt.Sprintf(string(DetailZeroMatch), n, query.PromDuration(window)),
			Action: Action{Label: LabelOpen, Intent: ActionIntent{Type: IntentRouteEdit, RouteID: id, Step: StepFilter}}})
	}
	return out, nil
}
