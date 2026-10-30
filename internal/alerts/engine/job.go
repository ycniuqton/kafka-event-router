package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/evaluator"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/platform/logging"
)

// LeaderGate tells whether this API instance is the scheduler leader (route-runtime's LeaderState).
type LeaderGate interface {
	IsLeader() bool
}

// EvalErrorCounter counts rules skipped because their query failed (router_alert_eval_errors_total).
type EvalErrorCounter interface {
	EvalError()
}

// PendingDispatcher delivers queued notifications (notify.NotificationDispatcher).
type PendingDispatcher interface {
	DispatchPending(ctx context.Context) error
}

// LogAttr names job log attributes.
type LogAttr string

const AttrRuleID LogAttr = "rule_id"

// AlertEvaluatorJob evaluates every enabled rule once per interval on the leader, then dispatches notifications.
type AlertEvaluatorJob struct {
	Leader     LeaderGate
	Rules      alerts.RuleRepository
	Routes     alerts.RouteDirectory
	Evaluators *evaluator.EvaluatorManager
	Querier    query.Querier
	Tracker    *StateTracker
	Dispatcher PendingDispatcher
	Errors     EvalErrorCounter
	Interval   time.Duration
	Log        *slog.Logger
	Defaults   *DefaultRuleProvisioner
}

// Run ticks until ctx ends.
func (j *AlertEvaluatorJob) Run(ctx context.Context) {
	ticker := time.NewTicker(j.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !j.Leader.IsLeader() {
				continue
			}
			if err := j.Tick(ctx); err != nil {
				j.Log.Warn("alerts: evaluation tick failed", string(logging.AttrError), err)
			}
		}
	}
}

// Tick provisions default rules, evaluates all enabled rules of live routes and dispatches pending notifications.
func (j *AlertEvaluatorJob) Tick(ctx context.Context) error {
	if j.Defaults != nil {
		j.Defaults.Ensure(ctx)
	}
	rules, err := j.Rules.ListEnabled(ctx)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.RouteID)
	}
	routes, err := j.Routes.Routes(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rules {
		route, ok := routes[r.RouteID]
		if !ok || route.Archived {
			continue
		}
		j.evaluate(ctx, r, route)
	}
	return j.Dispatcher.DispatchPending(ctx)
}

// evaluate queries one rule and applies the result; failures skip the rule for this tick.
func (j *AlertEvaluatorJob) evaluate(ctx context.Context, rule alerts.AlertRule, route alerts.RouteInfo) {
	ev, err := j.Evaluators.For(rule.Kind)
	if err != nil {
		j.fail(rule, err)
		return
	}
	vec, err := j.Querier.Instant(ctx, ev.Expr(rule))
	if err != nil {
		j.fail(rule, err)
		return
	}
	sample := evaluator.SampleReader{}.Of(vec, route.Running)
	obs := Observation{Rule: rule, Route: route, Breached: ev.Breached(sample, rule)}
	if sample.Present {
		obs.Value = &sample.Value
	}
	if _, err := j.Tracker.Apply(ctx, obs); err != nil {
		j.fail(rule, err)
	}
}

func (j *AlertEvaluatorJob) fail(rule alerts.AlertRule, err error) {
	j.Errors.EvalError()
	j.Log.Warn("alerts: rule evaluation skipped", string(AttrRuleID), rule.ID.String(), string(logging.AttrError), err)
}
