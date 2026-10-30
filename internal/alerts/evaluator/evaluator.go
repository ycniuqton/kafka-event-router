// Package evaluator turns alert rules into PromQL and decides whether a sample breaches the rule.
// One RuleEvaluator per AlertRuleKind, registered in the EvaluatorManager at boot (no switch on kinds).
package evaluator

import (
	"fmt"
	"math"
	"time"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/metrics/query"
)

// Sample is the evaluated value of one rule: Present is false when the metrics store returned no series.
type Sample struct {
	Value        float64
	Present      bool
	RouteRunning bool
}

// SampleReader turns instant vectors into samples.
type SampleReader struct{}

// Of returns the first element as a sample (no element, NaN or Inf counts as absent).
func (SampleReader) Of(v query.Vector, running bool) Sample {
	if len(v) == 0 || math.IsNaN(v[0].Value) || math.IsInf(v[0].Value, 0) {
		return Sample{RouteRunning: running}
	}
	return Sample{Value: v[0].Value, Present: true, RouteRunning: running}
}

// RuleEvaluator is the base of every rule kind.
type RuleEvaluator interface {
	Kind() alerts.AlertRuleKind
	Expr(rule alerts.AlertRule) query.Expr
	Breached(s Sample, rule alerts.AlertRule) bool
}

// LagResolution is the subquery step of the lag rule.
type LagResolution time.Duration

const DefaultLagResolution LagResolution = LagResolution(30 * time.Second)

// ErrUnknownKind reports a rule kind with no registered evaluator.
type ErrUnknownKind struct {
	Kind alerts.AlertRuleKind
}

func (e ErrUnknownKind) Error() string {
	return fmt.Sprintf("alerts: no evaluator for kind %q", e.Kind)
}

// EvaluatorManager is the registry AlertRuleKind → RuleEvaluator.
type EvaluatorManager struct {
	byKind map[alerts.AlertRuleKind]RuleEvaluator
}

// NewEvaluatorManager registers evaluators (later registrations of a kind win).
func NewEvaluatorManager(evaluators ...RuleEvaluator) *EvaluatorManager {
	m := &EvaluatorManager{byKind: map[alerts.AlertRuleKind]RuleEvaluator{}}
	for _, e := range evaluators {
		m.byKind[e.Kind()] = e
	}
	return m
}

// NewDefaultEvaluatorManager registers the built-in kinds.
func NewDefaultEvaluatorManager() *EvaluatorManager {
	return NewEvaluatorManager(LagAboveEvaluator{}, ErrorRateEvaluator{}, ThroughputZeroEvaluator{}, MatchRateChangeEvaluator{},
		RouterCausedEvaluator{})
}

// For returns the evaluator of kind.
func (m *EvaluatorManager) For(kind alerts.AlertRuleKind) (RuleEvaluator, error) {
	e, ok := m.byKind[kind]
	if !ok {
		return nil, ErrUnknownKind{Kind: kind}
	}
	return e, nil
}

// RuleQuery scopes a rule's query to its route and window.
type RuleQuery struct{}

// Params returns the expression parameters of rule.
func (RuleQuery) Params(rule alerts.AlertRule) query.Params {
	return query.Params{RouteIDs: []string{rule.RouteID.String()}, Range: rule.Window.Duration(),
		Step: time.Duration(DefaultLagResolution)}
}

// LagAboveEvaluator fires when the route's total lag stayed above N for the whole window.
type LagAboveEvaluator struct{}

func (LagAboveEvaluator) Kind() alerts.AlertRuleKind { return alerts.KindLagAbove }

func (LagAboveEvaluator) Expr(rule alerts.AlertRule) query.Expr {
	return query.ExprCatalog{}.Build(query.QueryAlertLagMin, RuleQuery{}.Params(rule))
}

func (LagAboveEvaluator) Breached(s Sample, rule alerts.AlertRule) bool {
	return s.Present && s.Value > rule.Threshold
}

// ErrorRateEvaluator fires when dead-lettered / consumed exceeds X % over the window.
type ErrorRateEvaluator struct{}

func (ErrorRateEvaluator) Kind() alerts.AlertRuleKind { return alerts.KindErrorRateAbove }

func (ErrorRateEvaluator) Expr(rule alerts.AlertRule) query.Expr {
	return query.ExprCatalog{}.Build(query.QueryAlertErrorRate, RuleQuery{}.Params(rule))
}

func (ErrorRateEvaluator) Breached(s Sample, rule alerts.AlertRule) bool {
	return s.Present && s.Value > rule.Threshold
}

// ThroughputZeroEvaluator fires when a running route produced nothing over the window (no series counts as zero).
type ThroughputZeroEvaluator struct{}

func (ThroughputZeroEvaluator) Kind() alerts.AlertRuleKind { return alerts.KindThroughputZero }

func (ThroughputZeroEvaluator) Expr(rule alerts.AlertRule) query.Expr {
	return query.ExprCatalog{}.Build(query.QueryProducedIncrease, RuleQuery{}.Params(rule))
}

func (ThroughputZeroEvaluator) Breached(s Sample, _ alerts.AlertRule) bool {
	return s.RouteRunning && (!s.Present || s.Value <= 0)
}

// MatchRateChangeEvaluator fires when the match rate moved by more than X points vs. the previous window.
type MatchRateChangeEvaluator struct{}

func (MatchRateChangeEvaluator) Kind() alerts.AlertRuleKind { return alerts.KindMatchRateChange }

func (MatchRateChangeEvaluator) Expr(rule alerts.AlertRule) query.Expr {
	return query.ExprCatalog{}.Build(query.QueryAlertMatchDelta, RuleQuery{}.Params(rule))
}

func (MatchRateChangeEvaluator) Breached(s Sample, rule alerts.AlertRule) bool {
	return s.Present && s.Value > rule.Threshold
}

// RouterCausedEvaluator fires when more than N dead letters of the router class (our bugs: internal errors,
// sandbox crashes, encode/produce/config failures, unreadable records) were written in the window.
type RouterCausedEvaluator struct{}

func (RouterCausedEvaluator) Kind() alerts.AlertRuleKind { return alerts.KindRouterCausedDLQ }

func (RouterCausedEvaluator) Expr(rule alerts.AlertRule) query.Expr {
	p := RuleQuery{}.Params(rule)
	for _, t := range record.TypesOf(record.ClassRouter) {
		p.ErrorTypes = append(p.ErrorTypes, string(t))
	}
	return query.ExprCatalog{}.Build(query.QueryAlertRouterDLQ, p)
}

func (RouterCausedEvaluator) Breached(s Sample, rule alerts.AlertRule) bool {
	return s.Present && s.Value > rule.Threshold
}
