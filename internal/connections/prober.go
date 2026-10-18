package connections

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/pkg/kafka"
)

// LeaderGate tells whether this replica runs singleton jobs (route-runtime#RR-40 provides leases).
type LeaderGate interface {
	IsLeader(ctx context.Context) bool
}

// AlwaysLeader runs the job on every replica until the lease-based gate exists (probes are idempotent).
type AlwaysLeader struct{}

// IsLeader implements LeaderGate.
func (AlwaysLeader) IsLeader(context.Context) bool { return true }

// HealthEvent is emitted when a probe changes a connection's health (`connection.health_changed`).
type HealthEvent struct {
	ConnectionID string       `json:"connectionId"`
	TeamID       string       `json:"teamId"`
	Previous     HealthStatus `json:"previous"`
	Current      HealthStatus `json:"current"`
	CheckedAt    time.Time    `json:"checkedAt"`
	LastFailure  *LastFailure `json:"lastFailure,omitempty"`
}

// HealthEventPublisher delivers health changes (monitoring#P2-BE-14 provides the WS/Redis publisher).
type HealthEventPublisher interface {
	Publish(ctx context.Context, e HealthEvent) error
}

// LogEvent names prober log events.
type LogEvent string

const (
	EventHealthChanged LogEvent = "connection.health_changed"
	EventProbeFailed   LogEvent = "connection.probe_failed"
)

// LogAttr names prober log attributes.
type LogAttr string

const (
	AttrConnectionID LogAttr = "connection_id"
	AttrPrevious     LogAttr = "previous"
	AttrCurrent      LogAttr = "current"
)

// LogHealthPublisher logs health changes until the monitoring publisher is wired.
type LogHealthPublisher struct {
	Log *slog.Logger
}

// Publish implements HealthEventPublisher.
func (p LogHealthPublisher) Publish(_ context.Context, e HealthEvent) error {
	p.Log.Info(string(EventHealthChanged), string(AttrConnectionID), e.ConnectionID, string(AttrPrevious), string(e.Previous), string(AttrCurrent), string(e.Current))
	return nil
}

// ProberConfig holds the probe interval, pool size, batch size and plan.
type ProberConfig struct {
	Interval time.Duration
	Workers  int
	Batch    int
	Plan     CheckPlan
}

// ProberDefaults are the default probe settings.
type ProberDefaults int

const (
	DefaultProbeWorkers ProberDefaults = 8
	DefaultProbeBatch   ProberDefaults = 200
)

// ProberDeps are the collaborators of the prober.
type ProberDeps struct {
	Repo      Repository
	Vault     *SecretVault
	Clients   ClientOpener
	Runner    *CheckRunner
	Leader    LeaderGate
	Publisher HealthEventPublisher
	Log       *slog.Logger
	Clock     Clock
}

// HealthProber re-checks every opted-in connection each interval and records health changes.
type HealthProber struct {
	d   ProberDeps
	cfg ProberConfig
}

// NewHealthProber builds the prober.
func NewHealthProber(d ProberDeps, cfg ProberConfig) *HealthProber {
	if d.Clock == nil {
		d.Clock = time.Now
	}
	return &HealthProber{d: d, cfg: cfg}
}

// Run ticks until ctx is cancelled.
func (p *HealthProber) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Tick(ctx)
		}
	}
}

// Tick probes every due connection with a bounded worker pool.
func (p *HealthProber) Tick(ctx context.Context) {
	if !p.d.Leader.IsLeader(ctx) {
		return
	}
	due, err := p.d.Repo.ListForProbe(ctx, p.d.Clock().Add(-p.cfg.Interval), p.cfg.Batch)
	if err != nil {
		p.d.Log.Warn(string(EventProbeFailed), string(logging.AttrError), err)
		return
	}
	slots := make(chan struct{}, max(p.cfg.Workers, 1))
	var wg sync.WaitGroup
	for _, c := range due {
		slots <- struct{}{}
		wg.Add(1)
		go func(c Connection) {
			defer func() { <-slots; wg.Done() }()
			p.probe(ctx, c)
		}(c)
	}
	wg.Wait()
}

// probe runs the probe plan for one connection and stores the outcome.
func (p *HealthProber) probe(ctx context.Context, c Connection) {
	result, err := p.check(ctx, c)
	if err != nil {
		p.d.Log.Warn(string(EventProbeFailed), string(AttrConnectionID), c.ID.String(), string(logging.AttrError), err)
		return
	}
	health := Health{Status: result.Health, CheckedAt: &result.CheckedAt, LastFailure: result.LastFailure}
	if err := p.d.Repo.UpdateHealth(ctx, c.ID, health); err != nil {
		p.d.Log.Warn(string(EventProbeFailed), string(AttrConnectionID), c.ID.String(), string(logging.AttrError), err)
		return
	}
	if result.Health == c.Health.Status {
		return
	}
	event := HealthEvent{ConnectionID: c.ID.String(), TeamID: c.TeamID.String(), Previous: c.Health.Status, Current: result.Health, CheckedAt: result.CheckedAt, LastFailure: result.LastFailure}
	if err := p.d.Publisher.Publish(ctx, event); err != nil {
		p.d.Log.Warn(string(EventProbeFailed), string(AttrConnectionID), c.ID.String(), string(logging.AttrError), err)
	}
}

func (p *HealthProber) check(ctx context.Context, c Connection) (TestResult, error) {
	bundle, err := p.d.Vault.Load(ctx, c.SecretRef)
	if err != nil {
		return TestResult{}, err
	}
	client, err := p.d.Clients.NewClient(ctx, c.Spec(bundle), kafka.PurposeAdmin)
	if err != nil {
		return TestResult{}, err
	}
	defer client.Close()
	session := &TestSession{Spec: c.Spec(bundle), Client: client}
	return p.d.Runner.Run(ctx, p.cfg.Plan, session, &CollectingSink{}), nil
}
