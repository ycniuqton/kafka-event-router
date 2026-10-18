package validation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// RunDefault holds the defaults of a validation run (design §5.4).
type RunDefault int

const (
	DefaultParallelism RunDefault = 4
	DefaultTimeoutMs   RunDefault = 5000
)

// ManagerOptions bound a validation run.
type ManagerOptions struct {
	Parallelism int
	Timeout     time.Duration
}

// withDefaults fills unset options.
func (o ManagerOptions) withDefaults() ManagerOptions {
	if o.Parallelism <= 0 {
		o.Parallelism = int(DefaultParallelism)
	}
	if o.Timeout <= 0 {
		o.Timeout = time.Duration(DefaultTimeoutMs) * time.Millisecond
	}
	return o
}

// Request is what callers pass to Run.
type Request struct {
	Mode      ValidationMode
	TeamID    string
	RouteID   string
	RouteName string
	Raw       []byte
}

// Result is the outcome of a run; Valid means no error-severity issue.
type Result struct {
	Valid  bool          `json:"valid"`
	Issues config.Issues `json:"issues"`
}

// ValidationManager runs the validators of a mode in parallel, each under a timeout.
type ValidationManager struct {
	set  *ValidatorSet
	opts ManagerOptions
}

// NewValidationManager builds the manager over a validator set.
func NewValidationManager(set *ValidatorSet, opts ManagerOptions) *ValidationManager {
	return &ValidationManager{set: set, opts: opts.withDefaults()}
}

// Run validates req.Raw for req.Mode; issues are sorted by path.
func (m *ValidationManager) Run(ctx context.Context, req Request) Result {
	in := InputBuilder{}.Build(req)
	validators := m.set.For(req.Mode)
	results := make([][]config.Issue, len(validators))
	sem := make(chan struct{}, m.opts.Parallelism)
	var wg sync.WaitGroup
	for i, v := range validators {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i] = m.runOne(ctx, v, in)
		}()
	}
	wg.Wait()
	issues := IssueOwnership{}.Merge(validators, results).Sort()
	return Result{Valid: !issues.HasErrors(), Issues: issues}
}

// runOne runs v under the per-validator timeout; failures become a "check skipped" warning.
func (m *ValidationManager) runOne(ctx context.Context, v ContextValidator, in ValidationInput) []config.Issue {
	cctx, cancel := context.WithTimeout(ctx, m.opts.Timeout)
	defer cancel()
	done := make(chan outcome, 1)
	go func() {
		issues, err := v.Validate(cctx, in)
		done <- outcome{issues: issues, err: err}
	}()
	select {
	case o := <-done:
		if o.err != nil {
			return []config.Issue{SkippedCheck{}.Issue(v.Name(), o.err)}
		}
		return o.issues
	case <-cctx.Done():
		return []config.Issue{SkippedCheck{}.Issue(v.Name(), cctx.Err())}
	}
}

// outcome carries one validator's result across the goroutine boundary.
type outcome struct {
	issues []config.Issue
	err    error
}

// SkippedCheck renders the warning for a validator that could not complete.
type SkippedCheck struct{}

// Issue returns the "check skipped" warning at the document root.
func (SkippedCheck) Issue(name ValidatorName, err error) config.Issue {
	reason := "it failed"
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "it timed out"
	}
	return config.NewWarning(IssueCheckSkipped, config.PointerRoot, "check skipped: %s (%s). Deploy is still allowed.", name, reason)
}

// InputBuilder parses the raw document once for every validator.
type InputBuilder struct{}

// Build parses raw; Doc and Config stay nil when the document cannot be parsed or decoded.
func (InputBuilder) Build(req Request) ValidationInput {
	in := ValidationInput{Mode: req.Mode, TeamID: req.TeamID, RouteID: req.RouteID, RouteName: req.RouteName, Raw: req.Raw}
	doc, err := config.Parse(req.Raw)
	if err != nil {
		return in
	}
	in.Doc = doc
	if cfg, err := doc.Config(); err == nil {
		in.Config = cfg
	}
	return in
}

// IssueOwner is a validator that is the authority for some issue codes: while it completes, the same
// codes reported by other validators are dropped (e.g. library refs checked against the real library).
type IssueOwner interface {
	OwnsIssues() []config.IssueCode
}

// IssueOwnership merges validator results, applying IssueOwner supersession.
type IssueOwnership struct{}

// Merge concatenates results in registration order without superseded issues.
func (o IssueOwnership) Merge(validators []ContextValidator, results [][]config.Issue) config.Issues {
	owners := o.owners(validators, results)
	issues := config.Issues{}
	for i, r := range results {
		for _, is := range r {
			if owner, owned := owners[is.Code]; !owned || owner == i {
				issues = append(issues, is)
			}
		}
	}
	return issues
}

// owners maps each owned code to the index of the completed validator owning it.
func (IssueOwnership) owners(validators []ContextValidator, results [][]config.Issue) map[config.IssueCode]int {
	out := map[config.IssueCode]int{}
	for i, v := range validators {
		owner, ok := v.(IssueOwner)
		if !ok || (SkippedCheck{}).In(results[i]) {
			continue
		}
		for _, code := range owner.OwnsIssues() {
			out[code] = i
		}
	}
	return out
}

// In reports whether issues contain a "check skipped" warning (the validator did not complete).
func (SkippedCheck) In(issues []config.Issue) bool {
	for _, is := range issues {
		if is.Code == IssueCheckSkipped {
			return true
		}
	}
	return false
}
