// Package retrytopic is the delayed auto-retry of transient failures (dead-letters 00001 §6): a route's
// errorPolicy.retry diverts eligible failures to one tier topic per attempt instead of blocking the source
// partition; a per-runner consumer reprocesses each tier record once it is due. Retries are at-least-once,
// also for exactly_once routes: a tier record is reprocessed and delivered outside the route transaction.
package retrytopic

import (
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
)

// BackoffSchedule is the delay of each attempt: delay(n) = min(base · 2^(n−1), max), n ≥ 1.
type BackoffSchedule struct {
	Base time.Duration
	Max  time.Duration
}

// Delay returns the wait of attempt n (attempts below 1 count as 1).
func (s BackoffSchedule) Delay(n int) time.Duration {
	d := s.Base
	for i := 1; i < n && d < s.Max; i++ {
		d *= 2
	}
	return min(d, s.Max)
}

// EligibleCategories are the transient processing failures a retry policy diverts to tier topics.
// Produce/destination outages are not listed: they keep the blocking partition backoff.
var EligibleCategories = map[domain.ErrorCategory]bool{
	domain.CategoryStepTimeout:          true,
	domain.CategoryDependencyTransient:  true,
	domain.CategoryDestinationTransient: true,
}

// Decision is what happens to a failed record under the retry policy.
type Decision string

const (
	DecisionNone      Decision = "none"
	DecisionSchedule  Decision = "schedule"
	DecisionExhausted Decision = "exhausted"
)

// Planner applies a route's resolved retry policy to failures.
type Planner struct {
	Policy config.ResolvedRetryPolicy
}

// NewPlanner resolves the retry policy of an error policy.
func NewPlanner(ep *config.ErrorPolicy) Planner {
	return Planner{Policy: config.RetryPolicyOf(ep)}
}

// Enabled reports whether the route retries through tier topics.
func (p Planner) Enabled() bool { return p.Policy.Enabled && p.Policy.MaxAttempts > 0 }

// Diverts reports whether a failure of category c goes to a tier topic instead of the in-place path.
func (p Planner) Diverts(c domain.ErrorCategory) bool { return p.Enabled() && EligibleCategories[c] }

// After decides the next step of a record of category c that already used `used` delayed attempts.
func (p Planner) After(used int, c domain.ErrorCategory) Decision {
	if !p.Diverts(c) {
		return DecisionNone
	}
	if used < p.Policy.MaxAttempts {
		return DecisionSchedule
	}
	return DecisionExhausted
}

// Schedule returns the backoff schedule of the policy.
func (p Planner) Schedule() BackoffSchedule {
	return BackoffSchedule{Base: p.Policy.BaseDelay, Max: p.Policy.MaxDelay}
}

// DueAt returns when attempt n scheduled at now becomes due.
func (p Planner) DueAt(now time.Time, n int) time.Time {
	return now.Add(p.Schedule().Delay(n))
}
