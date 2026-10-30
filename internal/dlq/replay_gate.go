package dlq

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq/record"
)

// GateDecision is what the 4-eyes gate says about a replay by the caller in ctx.
type GateDecision string

const (
	GateAllow           GateDecision = "allow"
	GateRequireApproval GateDecision = "require_approval"
	GateDeny            GateDecision = "deny"
)

// ReplayGate is the strict 4-eyes port (design 00001 §4.5): replays reach the active version's destinations,
// so they need the same second person a deploy to those destinations needs. cmd/api adapts approvals.Manager.Gate.
type ReplayGate interface {
	Check(ctx context.Context, target RetryTarget) (GateDecision, error)
}

// AlwaysAllowGate allows every replay (tests, deployments without approvals).
type AlwaysAllowGate struct{}

// Check implements ReplayGate.
func (AlwaysAllowGate) Check(context.Context, RetryTarget) (GateDecision, error) {
	return GateAllow, nil
}

// replayGate returns the configured gate.
func (m *Manager) replayGate() ReplayGate {
	if m.d.Gate == nil {
		return AlwaysAllowGate{}
	}
	return m.d.Gate
}

// CheckReplay evaluates the gate for a route's active version (sync retry, job creation).
func (m *Manager) CheckReplay(ctx context.Context, route RouteInfo) (GateDecision, error) {
	target, err := m.retryTarget(ctx, route)
	if err != nil {
		return "", err
	}
	return m.replayGate().Check(ctx, *target)
}

// GateError maps a non-allow decision to its problem.
type GateError struct{}

// Of returns nil for allow, ErrApprovalRequired or ErrReplayDenied otherwise.
func (GateError) Of(d GateDecision) error {
	switch d {
	case GateRequireApproval:
		return ErrApprovalRequired
	case GateDeny:
		return ErrReplayDenied
	}
	return nil
}

// DryRunReport is POST …/dlq/dry-run: a sample of the selection run through the active version.
type DryRunReport struct {
	ActiveVersionNo int                `json:"activeVersionNo"`
	Sampled         int                `json:"sampled"`
	Total           int                `json:"total"`
	Results         []DryRunResult     `json:"results"`
	Counts          map[DryOutcome]int `json:"counts"`
	Masked          bool               `json:"masked"`
}

// DryRun previews a selection (≤ DryRunSample newest records) without producing (design 00001 §4.3).
func (m *Manager) DryRun(ctx context.Context, viewer Viewer, routeID string, sel Selection) (DryRunReport, error) {
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return DryRunReport{}, err
	}
	target, err := m.retryTarget(ctx, loc.Route)
	if err != nil {
		return DryRunReport{}, err
	}
	if len(sel.IDs) > int(DryRunSample) {
		sel.IDs = sel.IDs[:DryRunSample]
	}
	resolved, err := m.selector.Resolve(ctx, loc, sel, DryRunCap)
	if err != nil {
		return DryRunReport{}, err
	}
	rc := ReplayContext{Location: loc, Target: target, Viewer: viewer}
	results, err := m.dryRun(ctx, rc, resolved.Records)
	if err != nil {
		return DryRunReport{}, err
	}
	report := DryRunReport{ActiveVersionNo: target.Version.No, Sampled: len(results), Total: len(results) + resolved.Remaining, Results: results}
	report.Counts, report.Masked = (DryCounts{}).Of(results), m.anyMasked(ctx, viewer, loc)
	detail := PreviewAuditDetail{IDs: sel.IDs, Match: AuditDetailOf{}.Match(sel), Counts: report.Counts, Masked: report.Masked, Version: target.Version.No}
	return report, m.d.Audit.Record(ctx, AuditEvent{Action: ActionDLQReplayPreview, Route: loc.Route, Detail: detail, Masked: report.Masked})
}

// dryRun runs records through the configured executor's DryRunner and masks outputs for masked viewers.
func (m *Manager) dryRun(ctx context.Context, rc ReplayContext, recs []record.Record) ([]DryRunResult, error) {
	runner, ok := m.actions[ActionRetry].(DryRunCapable)
	if !ok {
		return nil, errNoDryRun
	}
	dry, ok := runner.DryRunner()
	if !ok {
		return nil, errNoDryRun
	}
	results := dry.DryRun(ctx, *rc.Target, recs)
	if !m.anyMasked(ctx, rc.Viewer, rc.Location) {
		return results, nil
	}
	return (DryMasker{Masker: PayloadMasker{Masker: TypePreservingMasker{}}}).All(results), nil
}

// anyMasked reports whether the viewer sees any DLQ connection of loc masked.
func (m *Manager) anyMasked(ctx context.Context, viewer Viewer, loc Location) bool {
	for _, t := range loc.Targets {
		if _, masked, err := m.d.Views.PresentRecord(ctx, viewer, t.ConnectionID, record.Record{}); err != nil || masked {
			return true
		}
	}
	return false
}

// DryRunCapable is implemented by action handlers whose executor can dry-run (RetryAction).
type DryRunCapable interface {
	DryRunner() (DryRunner, bool)
}

// DryRunner implements DryRunCapable.
func (a RetryAction) DryRunner() (DryRunner, bool) {
	d, ok := a.Executor.(DryRunner)
	return d, ok
}

// DryCounts counts dry-run outcomes.
type DryCounts struct{}

// Of returns a count per outcome (every outcome present).
func (DryCounts) Of(results []DryRunResult) map[DryOutcome]int {
	out := map[DryOutcome]int{}
	for _, o := range DryOutcomes {
		out[o] = 0
	}
	for _, r := range results {
		out[r.Outcome]++
	}
	return out
}

// DryMasker masks dry-run outputs like the source view (type-preserving).
type DryMasker struct {
	Masker PayloadMasker
}

// All masks every output of results.
func (d DryMasker) All(results []DryRunResult) []DryRunResult {
	for i := range results {
		for j := range results[i].Outputs {
			results[i].Outputs[j] = d.one(results[i].Outputs[j])
		}
	}
	return results
}

func (d DryMasker) one(o OutputView) OutputView {
	o.Value = d.Masker.Encoded(o.Value)
	if o.Key != nil {
		k := d.Masker.Encoded(*o.Key)
		o.Key = &k
	}
	for i, h := range o.Headers {
		o.Headers[i].Value = d.Masker.Text(h.Value)
	}
	return o
}

// Workflow audit actions (feature-owned, registered in the catalog at wiring, design 00001 §8).
const (
	ActionDLQEditedReplayed audit.Action = "dlq.edited_replayed"
	ActionDLQReplayRequest  audit.Action = "dlq.replay_requested"
	ActionDLQReplayApproved audit.Action = "dlq.replay_approved"
	ActionDLQReplayRejected audit.Action = "dlq.replay_rejected"
	ActionDLQReplayPreview  audit.Action = "dlq.replay_previewed"
)

// PreviewAuditDetail is the diff stored for a dry run (ids and counts only).
type PreviewAuditDetail struct {
	IDs     []string           `json:"ids,omitempty"`
	Match   *MatchAudit        `json:"match,omitempty"`
	Counts  map[DryOutcome]int `json:"counts"`
	Masked  bool               `json:"masked"`
	Edited  bool               `json:"edited,omitempty"`
	Version int                `json:"versionNo"`
}

// EditAuditDetail is the diff stored for an edited replay: changed paths, never values.
type EditAuditDetail struct {
	ID        string          `json:"id"`
	Diff      []DiffEntry     `json:"diff"`
	Note      string          `json:"note,omitempty"`
	Counts    map[Outcome]int `json:"counts,omitempty"`
	Masked    bool            `json:"masked"`
	VersionNo int             `json:"versionNo,omitempty"`
	JobID     string          `json:"jobId,omitempty"`
}
