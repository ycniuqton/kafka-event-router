package connections

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/pkg/kafka"
)

// PreparedTest is a validated test target, ready to stream.
type PreparedTest struct {
	Connection Connection
	Spec       kafka.ConnectionSpec
	Saved      bool
}

// TesterDeps are the collaborators of the tester.
type TesterDeps struct {
	Repo      Repository
	Resolver  *SpecResolver
	Vault     *SecretVault
	Validator *Validator
	Clients   ClientOpener
	Runner    *CheckRunner
	Plan      CheckPlan
}

// Tester runs the test checklist for saved connections and unsaved drafts.
type Tester struct {
	d       TesterDeps
	auditor Auditor
}

// NewTester builds the tester.
func NewTester(d TesterDeps) *Tester {
	return &Tester{d: d}
}

// PrepareSaved resolves a stored connection (before the stream opens, so errors are problems).
func (t *Tester) PrepareSaved(ctx context.Context, id uuid.UUID) (PreparedTest, error) {
	c, spec, err := t.d.Resolver.Resolve(ctx, id)
	if err != nil {
		return PreparedTest{}, err
	}
	return PreparedTest{Connection: c, Spec: spec, Saved: true}, nil
}

// PrepareDraft validates a draft; with a stored connection id, missing secrets come from storage.
func (t *Tester) PrepareDraft(ctx context.Context, d Draft, stored *uuid.UUID) (PreparedTest, error) {
	target := Connection{TeamID: d.TeamID, Name: d.Name}
	if stored != nil {
		c, spec, err := t.d.Resolver.Resolve(ctx, *stored)
		if err != nil {
			return PreparedTest{}, err
		}
		d.Secrets = spec.Secret.Merge(d.Secrets)
		target = c
	}
	if err := t.d.Validator.Validate(ctx, d); err != nil {
		return PreparedTest{}, err
	}
	spec := d.Spec()
	spec.ID = target.ID.String()
	return PreparedTest{Connection: target, Spec: spec}, nil
}

// Execute runs the plan, streaming to sink; saved tests update health. Cancelled runs change nothing.
func (t *Tester) Execute(ctx context.Context, p PreparedTest, opts TestOptions, sink TestSink) error {
	client, err := t.d.Clients.NewClient(ctx, p.Spec, kafka.PurposeAdmin)
	if err != nil {
		return err
	}
	defer client.Close()
	session := &TestSession{Spec: p.Spec, Client: client, Options: opts}
	result := t.d.Runner.Run(ctx, t.d.Plan, session, sink)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p.Saved {
		health := Health{Status: result.Health, CheckedAt: &result.CheckedAt, LastFailure: result.LastFailure}
		if err := t.d.Repo.UpdateHealth(ctx, p.Connection.ID, health); err != nil {
			return err
		}
	}
	return t.auditor.Record(ctx, ActionTested, p.Connection, nil, TestAudit{OK: result.OK, Health: result.Health, Draft: !p.Saved})
}
