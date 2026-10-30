package dlq

import (
	"context"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq/record"
)

// ReplayLimit bounds dry-runs and notes (design 00001 §4).
type ReplayLimit int

const (
	DryRunSample  ReplayLimit = 20
	MaxNoteLength ReplayLimit = 2000
)

// DryRunCap samples the newest records of a selection for a dry run.
var DryRunCap = SelectionCap{Limit: int(DryRunSample), Err: ErrSelectionTooBig, Truncate: true}

// ReplayRequest is POST …/{dlqId}/replay: an optional edit, dry-run or real replay, and a note.
type ReplayRequest struct {
	Edit   *RecordEdit
	DryRun bool
	Note   string
}

// ReplayStatus is how a real replay ended.
type ReplayStatus string

const (
	ReplayDone            ReplayStatus = "done"
	ReplayPendingApproval ReplayStatus = "pending_approval"
)

// ReplayResult is the outcome of Replay; Pending carries the planned (unmasked) edit the job stores.
type ReplayResult struct {
	DryRun          bool
	Status          ReplayStatus
	ActiveVersionNo int
	Masked          bool
	Diff            []DiffEntry
	Dry             *DryRunResult
	Report          *ActionReport
	Pending         *PendingReplay
}

// PendingReplay is a replay waiting for a second person: the route, the record and the planned edit.
type PendingReplay struct {
	Route  RouteInfo
	ID     string
	Edit   *RecordEdit
	Diff   []DiffEntry
	Note   string
	Edited bool
}

// Replay re-runs one dead letter, optionally edited, through the active version (design 00001 §4.2).
func (m *Manager) Replay(ctx context.Context, viewer Viewer, actor Actor, routeID, id string, req ReplayRequest) (ReplayResult, error) {
	if utf8.RuneCountInString(req.Note) > int(MaxNoteLength) {
		return ReplayResult{}, ErrInvalidNote.At(string(PathNote))
	}
	ctxR, err := m.replayContext(ctx, viewer, routeID, id)
	if err != nil {
		return ReplayResult{}, err
	}
	plan, err := m.planner().Plan(EditInput{Original: ctxR.Record, Edit: req.Edit, Masked: ctxR.Masked, Format: ctxR.Format})
	if err != nil {
		return ReplayResult{}, err
	}
	out := ReplayResult{DryRun: req.DryRun, ActiveVersionNo: ctxR.Target.Version.No, Masked: ctxR.Masked, Diff: plan.Diff}
	if req.DryRun {
		dry, err := m.dryRun(ctx, ctxR, []record.Record{plan.Record})
		if err != nil {
			return ReplayResult{}, err
		}
		out.Dry = &dry[0]
		detail := PreviewAuditDetail{IDs: []string{id}, Counts: DryCounts{}.Of(dry), Masked: ctxR.Masked, Edited: plan.Edited, Version: out.ActiveVersionNo}
		return out, m.d.Audit.Record(ctx, AuditEvent{Action: ActionDLQReplayPreview, Route: ctxR.Location.Route, Detail: detail, Masked: ctxR.Masked})
	}
	return m.replayNow(ctx, actor, ctxR, plan, req, out)
}

// replayNow runs the planned record, or parks it for approval when the 4-eyes gate requires it.
func (m *Manager) replayNow(ctx context.Context, actor Actor, rc ReplayContext, plan EditPlan, req ReplayRequest, out ReplayResult) (ReplayResult, error) {
	decision, err := m.replayGate().Check(ctx, *rc.Target)
	if err != nil {
		return ReplayResult{}, err
	}
	switch decision {
	case GateDeny:
		return ReplayResult{}, ErrReplayDenied
	case GateRequireApproval:
		out.Status = ReplayPendingApproval
		out.Pending = &PendingReplay{Route: rc.Location.Route, ID: rc.ID, Edit: (PlannedEdit{}).Of(plan), Diff: plan.Diff, Note: req.Note, Edited: plan.Edited}
		return out, nil
	}
	report := m.retryRecords(ctx, ActionContext{Location: rc.Location, Actor: actor, Target: rc.Target, Now: m.d.Now()}, []record.Record{plan.Record})
	out.Status, out.Report = ReplayDone, &report
	return out, m.auditReplay(ctx, rc, plan, req.Note, report)
}

// ReplayContext is everything one replay needs about its record.
type ReplayContext struct {
	ID       string
	Location Location
	Record   record.Record
	Target   *RetryTarget
	Masked   bool
	Format   SourceFormat
	Viewer   Viewer
}

// replayContext loads the record, the active version, the viewer's masking and the replaying source format.
func (m *Manager) replayContext(ctx context.Context, viewer Viewer, routeID, id string) (ReplayContext, error) {
	ref, err := RefCodec{}.Decode(id)
	if err != nil {
		return ReplayContext{}, ErrInvalidID.At(string(PathDLQID))
	}
	loc, rec, err := m.fetchOne(ctx, routeID, ref)
	if err != nil {
		return ReplayContext{}, err
	}
	target, err := m.retryTarget(ctx, loc.Route)
	if err != nil {
		return ReplayContext{}, err
	}
	conn, _ := loc.TargetOf(ref.Topic)
	_, masked, err := m.d.Views.PresentRecord(ctx, viewer, conn.ConnectionID, rec)
	if err != nil {
		return ReplayContext{}, err
	}
	format, ok := loc.Formats.Of(target.Version.No)
	if !ok {
		format, _ = loc.Formats.Of(rec.Failure.VersionNo)
	}
	return ReplayContext{ID: id, Location: loc, Record: rec, Target: target, Masked: masked, Format: format, Viewer: viewer}, nil
}

// retryRecords applies the retry handler to recs (lock, execute, mark) and refreshes counters.
func (m *Manager) retryRecords(ctx context.Context, ac ActionContext, recs []record.Record) ActionReport {
	report := m.actions[ActionRetry].Apply(ctx, ac, recs)
	m.d.Summaries.Invalidate(ctx, ac.Location.Route.ID)
	m.d.Metrics.Action(report)
	return report
}

// auditReplay writes dlq.edited_replayed (paths only) for edited records, else the plain dlq.retried.
func (m *Manager) auditReplay(ctx context.Context, rc ReplayContext, plan EditPlan, note string, report ActionReport) error {
	if !plan.Edited {
		detail := AuditDetailOf{}.Action(report, Selection{IDs: []string{rc.ID}})
		return m.d.Audit.Record(ctx, AuditEvent{Action: audit.ActionDLQRetried, Route: rc.Location.Route, Detail: detail})
	}
	detail := EditAuditDetail{ID: rc.ID, Diff: plan.Diff, Note: note, Counts: report.Counts(), Masked: plan.Masked, VersionNo: report.ActiveVersionNo}
	return m.d.Audit.Record(ctx, AuditEvent{Action: ActionDLQEditedReplayed, Route: rc.Location.Route, Detail: detail, Masked: plan.Masked})
}

// ReplayApproved runs an approved pending replay (job runner): the stored edit is already unmasked.
func (m *Manager) ReplayApproved(ctx context.Context, actor Actor, jobID, routeID, id string, edit *RecordEdit) (ActionReport, error) {
	rc, err := m.replayContext(ctx, Viewer{UserID: actor.UserID}, routeID, id)
	if err != nil {
		return ActionReport{}, err
	}
	plan, err := m.planner().Plan(EditInput{Original: rc.Record, Edit: edit, Format: rc.Format})
	if err != nil {
		return ActionReport{}, err
	}
	ac := ActionContext{Location: rc.Location, Actor: actor, Target: rc.Target, Now: m.d.Now(), JobID: jobID}
	return m.retryRecords(ctx, ac, []record.Record{plan.Record}), nil
}

// planner returns the edit planner with the masking policy's masker.
func (m *Manager) planner() EditPlanner {
	return EditPlanner{Masker: TypePreservingMasker{}}
}

// PlannedEdit stores a planned record as a full edit (key, value, headers) for a pending job.
type PlannedEdit struct{}

// Of returns the edit of plan, nil when nothing changed.
func (PlannedEdit) Of(plan EditPlan) *RecordEdit {
	if !plan.Edited {
		return nil
	}
	key, value := ValueEncoder{}.Encode(plan.Record.Key), ValueEncoder{}.Encode(plan.Record.Value)
	headers := make([]EncodedHeader, 0, len(plan.Record.Headers))
	for _, h := range plan.Record.Headers {
		enc := ValueEncoder{}.Encode(h.Value)
		text, _ := enc.Data.(string)
		if enc.Encoding != EncodingBase64 {
			enc.Encoding, text = EncodingUTF8, string(h.Value)
		}
		headers = append(headers, EncodedHeader{Key: h.Key, Value: text, Encoding: enc.Encoding})
	}
	return &RecordEdit{Key: &key, Value: &value, Headers: &headers}
}
