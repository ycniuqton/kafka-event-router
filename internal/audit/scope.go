package audit

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/google/uuid"
)

type scopeKey struct{}

// Scope is the per-request audit context; services call Record inside their transaction.
type Scope struct {
	request  RequestContext
	recorder Recorder
	diff     *DiffBuilder
	count    atomic.Int32
}

// NewScope builds a scope for one request.
func NewScope(rc RequestContext, recorder Recorder, diff *DiffBuilder) *Scope {
	return &Scope{request: rc, recorder: recorder, diff: diff}
}

// WithScope stores the scope in ctx.
func WithScope(ctx context.Context, s *Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// FromContext returns the request's scope, if the audit middleware installed one.
func FromContext(ctx context.Context) (*Scope, bool) {
	s, ok := ctx.Value(scopeKey{}).(*Scope)
	return s, ok
}

// Change describes one audited mutation recorded through a Scope.
type Change struct {
	Action   Action
	Object   ObjectType
	ObjectID string
	TeamID   *uuid.UUID
	Before   any
	After    any
}

// Record computes the redacted diff and writes the entry through the recorder (same tx as ctx).
func (s *Scope) Record(ctx context.Context, c Change) error {
	diff, err := s.diffOf(c)
	if err != nil {
		return err
	}
	entry := s.request.Apply(Entry{Action: c.Action, Object: c.Object, ObjectID: c.ObjectID, TeamID: c.TeamID, Diff: diff})
	if err := s.recorder.Record(ctx, entry); err != nil {
		return err
	}
	s.count.Add(1)
	return nil
}

// RecordEntry writes a pre-built entry (e.g. auth events) with the request context applied.
func (s *Scope) RecordEntry(ctx context.Context, e Entry) error {
	if err := s.recorder.Record(ctx, s.request.Apply(e)); err != nil {
		return err
	}
	s.count.Add(1)
	return nil
}

// Recorded returns how many entries this scope wrote.
func (s *Scope) Recorded() int {
	return int(s.count.Load())
}

// Request returns the request context the scope applies to entries.
func (s *Scope) Request() RequestContext {
	return s.request
}

func (s *Scope) diffOf(c Change) (json.RawMessage, error) {
	if c.Before == nil && c.After == nil {
		return nil, nil
	}
	return s.diff.Build(c.Before, c.After)
}
