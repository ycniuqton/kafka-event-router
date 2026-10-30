package dlq

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
)

// AuditEvent is one audited dead-letter action; Detail holds ids/counts only, never payloads.
type AuditEvent struct {
	Action  audit.Action
	Route   RouteInfo
	Detail  any
	Masked  bool
	Summary string
}

// AuditTrail records dead-letter actions.
type AuditTrail interface {
	Record(ctx context.Context, e AuditEvent) error
}

// ScopeAuditTrail writes through the request's audit scope (installed by the audit middleware,
// or by the export handler for its GET).
type ScopeAuditTrail struct{}

// Record implements AuditTrail; without a scope nothing is written.
func (ScopeAuditTrail) Record(ctx context.Context, e AuditEvent) error {
	scope, ok := audit.FromContext(ctx)
	if !ok {
		return nil
	}
	entry, err := AuditEntryOf{}.Event(e)
	if err != nil {
		return err
	}
	return scope.RecordEntry(ctx, entry)
}

// AuditEntryOf renders dead-letter events as audit_log rows: object type dlq, the route as object
// (id, name label, team), the detail as diff - the shape audit export and SIEM sinks ship (DL-203).
type AuditEntryOf struct{}

// Event returns the entry of e.
func (AuditEntryOf) Event(e AuditEvent) (audit.Entry, error) {
	diff, err := json.Marshal(e.Detail)
	if err != nil {
		return audit.Entry{}, err
	}
	entry := audit.Entry{Action: e.Action, Object: audit.ObjectDLQ, ObjectID: e.Route.ID, Diff: diff, ObjectLabel: e.Route.Name}
	if team, err := uuid.Parse(e.Route.TeamID); err == nil {
		entry.TeamID = &team
	}
	return entry, nil
}

// RecorderAuditTrail writes events outside a request (background jobs) as Actor.
type RecorderAuditTrail struct {
	Recorder audit.Recorder
	Actor    audit.Actor
}

// Record implements AuditTrail.
func (t RecorderAuditTrail) Record(ctx context.Context, e AuditEvent) error {
	entry, err := AuditEntryOf{}.Event(e)
	if err != nil {
		return err
	}
	entry.Actor = t.Actor
	return t.Recorder.Record(ctx, entry)
}

// AuditActorName names the system actors of the feature.
type AuditActorName string

const JobRunnerActor AuditActorName = "dlq job runner"

// NoAuditTrail drops events (tests).
type NoAuditTrail struct{}

// Record implements AuditTrail.
func (NoAuditTrail) Record(context.Context, AuditEvent) error { return nil }

// Audit actions the feature registers in the access-control catalog at wiring time (P2).
const (
	ActionDLQRestored     audit.Action = "dlq.restored"
	ActionDLQJobCreated   audit.Action = "dlq.job_created"
	ActionDLQJobCancelled audit.Action = "dlq.job_cancelled"
	ActionDLQJobFinished  audit.Action = "dlq.job_finished"
)

// CatalogActions lists the feature-owned audit actions with their catalog metadata.
var CatalogActions = map[audit.Action]audit.ActionMeta{
	ActionDLQRestored:       {Category: audit.CategoryChange},
	ActionDLQJobCreated:     {Category: audit.CategoryChange},
	ActionDLQJobCancelled:   {Category: audit.CategoryChange},
	ActionDLQJobFinished:    {Category: audit.CategoryChange},
	ActionDLQEditedReplayed: {Category: audit.CategoryChange},
	ActionDLQReplayRequest:  {Category: audit.CategoryChange},
	ActionDLQReplayApproved: {Category: audit.CategoryChange},
	ActionDLQReplayRejected: {Category: audit.CategoryChange},
	ActionDLQReplayPreview:  {Category: audit.CategoryAccess},
}

// AuditActions lists every dead-letter audit action (platform catalog and feature-owned), for exports and presets.
var AuditActions = []audit.Action{audit.ActionDLQViewed, audit.ActionDLQRetried, audit.ActionDLQDiscarded, audit.ActionDLQExported,
	ActionDLQRestored, ActionDLQJobCreated, ActionDLQJobCancelled, ActionDLQJobFinished,
	ActionDLQEditedReplayed, ActionDLQReplayRequest, ActionDLQReplayApproved, ActionDLQReplayRejected, ActionDLQReplayPreview}

// ActionAudit maps action kinds to catalogued audit actions.
var ActionAudit = map[ActionKind]audit.Action{
	ActionRetry:   audit.ActionDLQRetried,
	ActionDiscard: audit.ActionDLQDiscarded,
	ActionRestore: ActionDLQRestored,
}

// ActionAuditDetail is the diff stored for retry/discard.
type ActionAuditDetail struct {
	Counts    map[Outcome]int `json:"counts"`
	IDs       []string        `json:"ids,omitempty"`
	Match     *MatchAudit     `json:"match,omitempty"`
	VersionNo int             `json:"versionNo,omitempty"`
	Remaining int             `json:"remaining"`
	AtLeast   bool            `json:"deliveredAtLeastOnce,omitempty"`
}

// MatchAudit is the filter part of an audited selection.
type MatchAudit struct {
	ErrorTypes []string `json:"errorTypes,omitempty"`
	Since      string   `json:"since,omitempty"`
	Until      string   `json:"until,omitempty"`
	Step       string   `json:"step,omitempty"`
	VersionNo  int      `json:"versionNo,omitempty"`
	Text       string   `json:"q,omitempty"`
	Classes    []string `json:"errorClasses,omitempty"`
	Cause      string   `json:"fingerprint,omitempty"`
}

// ExportAuditDetail is the diff stored for export.
type ExportAuditDetail struct {
	Count  int          `json:"count"`
	Format ExportFormat `json:"format"`
	IDs    []string     `json:"ids,omitempty"`
	Match  *MatchAudit  `json:"match,omitempty"`
	Masked bool         `json:"masked"`
}

// ViewAuditDetail is the diff stored for a detail view (dlq.viewed).
type ViewAuditDetail struct {
	ID     string `json:"id"`
	Masked bool   `json:"masked"`
}
