package failures_test

import (
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/retrytopic"
	"github.com/eventrouter/router/pkg/engine"
)

type lookupCase struct {
	err       error
	category  domain.ErrorCategory
	dlqType   record.ErrorType
	class     record.ErrorClass
	retryable bool
}

func lookupCases() map[string]lookupCase {
	dep := &engine.DependencyError{Kind: engine.DependencyLookup, Name: "crm", Cause: errors.New("503")}
	return map[string]lookupCase{
		"unavailable": {err: &engine.ProcessError{Type: engine.ErrLookupUnavailable, Cause: dep},
			category: domain.CategoryDependencyTransient, dlqType: record.ErrorLookupUnavailable, class: record.ClassTransient, retryable: true},
		"miss": {err: &engine.ProcessError{Type: engine.ErrLookupMiss, Cause: errors.New("no answer")},
			category: domain.CategoryStep, dlqType: record.ErrorStep, class: record.ClassData},
	}
}

func TestLookupErrorsShareOneContract(t *testing.T) {
	for name, c := range lookupCases() {
		got := failures.DefaultClassifier{}.Classify(c.err)
		if got != c.category || (failures.DispositionTable{}).For(got) != domain.DispositionApplyPolicy {
			t.Fatalf("%s: category %s disposition %s", name, got, failures.DispositionTable{}.For(got))
		}
		et := failures.DeadLetterTypes{}.Of(failures.Failure{Err: c.err, Category: got})
		if et != c.dlqType || et.Class() != c.class {
			t.Fatalf("%s: dlq type %s class %s", name, et, et.Class())
		}
		if retrytopic.EligibleCategories[got] != c.retryable {
			t.Fatalf("%s: retry eligibility %v", name, retrytopic.EligibleCategories[got])
		}
	}
}
