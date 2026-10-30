package dlq

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq/record"
)

// Act applies retry or discard to a selection (design §5.5, §5.6): resolve, dispatch, audit.
func (m *Manager) Act(ctx context.Context, kind ActionKind, actor Actor, routeID string, sel Selection) (ActionReport, error) {
	handler, ok := m.actions[kind]
	if !ok {
		return ActionReport{}, fmt.Errorf("dlq: no action handler %q", kind)
	}
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return ActionReport{}, err
	}
	ac := ActionContext{Location: loc, Actor: actor, Now: m.d.Now()}
	if handler.NeedsActiveVersion() {
		if ac.Target, err = m.gatedTarget(ctx, loc.Route); err != nil {
			return ActionReport{}, err
		}
	}
	resolved, err := m.selector.Resolve(ctx, loc, (SelectionOf{}).For(handler, sel), ActionCap)
	if err != nil {
		return ActionReport{}, err
	}
	report := handler.Apply(ctx, ac, resolved.Records)
	report.Results = append(resolved.Skipped, report.Results...)
	report.Remaining = resolved.Remaining
	m.d.Summaries.Invalidate(ctx, routeID)
	m.d.Metrics.Action(report)
	return report, m.d.Audit.Record(ctx, AuditEvent{Action: ActionAudit[kind], Route: loc.Route, Detail: (AuditDetailOf{}).Action(report, sel)})
}

// SelectionOf adapts selections to an action.
type SelectionOf struct{}

// For makes match selections of actions on resolved records include resolved records.
func (SelectionOf) For(h ActionHandler, sel Selection) Selection {
	if sel.Match == nil || !h.SelectsResolved() {
		return sel
	}
	q := *sel.Match
	q.IncludeResolved = true
	sel.Match = &q
	return sel
}

// gatedTarget loads the active version and refuses sync replays the 4-eyes gate does not allow.
func (m *Manager) gatedTarget(ctx context.Context, route RouteInfo) (*RetryTarget, error) {
	target, err := m.retryTarget(ctx, route)
	if err != nil {
		return nil, err
	}
	decision, err := m.replayGate().Check(ctx, *target)
	if err != nil {
		return nil, err
	}
	return target, GateError{}.Of(decision)
}

// retryTarget loads the active version; an undeployed route cannot be retried.
func (m *Manager) retryTarget(ctx context.Context, route RouteInfo) (*RetryTarget, error) {
	if !route.HasActiveVersion() {
		return nil, ErrNoActiveVersion.At(string(PathActive))
	}
	v, err := m.d.Routes.Version(ctx, route.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	return &RetryTarget{Route: route, Version: v}, nil
}

// ExportFile is a ready export: the writer, the views and the attachment name.
type ExportFile struct {
	Exporter Exporter
	Views    []DetailView
	Name     string
}

// Export resolves a selection (≤ MaxExport) into detail views and audits it (design §5.8).
func (m *Manager) Export(ctx context.Context, viewer Viewer, routeID string, sel Selection, format ExportFormat) (ExportFile, error) {
	exporter, err := m.d.Exports.For(format)
	if err != nil {
		return ExportFile{}, err
	}
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return ExportFile{}, err
	}
	resolved, err := m.selector.Resolve(ctx, loc, sel, ExportCap)
	if err != nil {
		return ExportFile{}, err
	}
	views, err := m.views(ctx, viewer, loc, resolved.Records)
	if err != nil {
		return ExportFile{}, err
	}
	slug := strings.TrimPrefix(record.TopicNaming{}.Default(loc.Route.Name), string(record.DefaultTopicPrefix))
	name := fmt.Sprintf(string(exportFileName), slug, m.d.Now().UTC().Format(string(exportTimeLayout)), exporter.Format())
	masked := (ViewsOf{}).AnyMasked(views)
	detail := ExportAuditDetail{Count: len(views), Format: exporter.Format(), IDs: sel.IDs, Match: (AuditDetailOf{}).Match(sel), Masked: masked}
	return ExportFile{Exporter: exporter, Views: views, Name: name}, m.d.Audit.Record(ctx, AuditEvent{Action: audit.ActionDLQExported, Route: loc.Route, Detail: detail, Masked: masked})
}

// views renders records through the view policy.
func (m *Manager) views(ctx context.Context, viewer Viewer, loc Location, recs []record.Record) ([]DetailView, error) {
	resolutions, err := m.resolutions(ctx, loc, recs)
	if err != nil {
		return nil, err
	}
	out := make([]DetailView, 0, len(recs))
	for _, r := range recs {
		v, err := m.detail(ctx, viewer, loc, r, resolutions.Of(r.Ref))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ViewsOf inspects rendered views.
type ViewsOf struct{}

// AnyMasked reports whether any view was masked.
func (ViewsOf) AnyMasked(views []DetailView) bool {
	for _, v := range views {
		if v.Masked {
			return true
		}
	}
	return false
}

// AuditDetailOf builds audit diffs (ids and counts only).
type AuditDetailOf struct{}

// Action returns the retry/discard diff.
func (AuditDetailOf) Action(r ActionReport, sel Selection) ActionAuditDetail {
	return ActionAuditDetail{Counts: r.Counts(), IDs: sel.IDs, Match: AuditDetailOf{}.Match(sel), VersionNo: r.ActiveVersionNo,
		Remaining: r.Remaining, AtLeast: r.Delivery.DeliveredAtLeastOnce}
}

// Match returns the filter of a match selection.
func (AuditDetailOf) Match(sel Selection) *MatchAudit {
	if sel.Match == nil {
		return nil
	}
	out := &MatchAudit{Step: sel.Match.Step, VersionNo: sel.Match.VersionNo, Text: sel.Match.Text, Cause: sel.Match.Fingerprint}
	for _, t := range sel.Match.ErrorTypes {
		out.ErrorTypes = append(out.ErrorTypes, string(t))
	}
	for _, c := range sel.Match.Classes {
		out.Classes = append(out.Classes, string(c))
	}
	if !sel.Match.Since.IsZero() {
		out.Since = sel.Match.Since.Format(time.RFC3339)
	}
	if !sel.Match.Until.IsZero() {
		out.Until = sel.Match.Until.Format(time.RFC3339)
	}
	return out
}
