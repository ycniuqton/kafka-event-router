package failures

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/sandbox"
)

func TestDeadLetterTypesTable(t *testing.T) {
	lookup := &engine.DependencyError{Kind: engine.DependencyLookup, Name: "customers", Cause: context.DeadlineExceeded}
	registry := &engine.DependencyError{Kind: engine.DependencySchemaRegistry, Cause: errors.New("503")}
	cases := []struct {
		name string
		f    Failure
		want record.ErrorType
	}{
		{"panic", Failure{Err: &engine.ProcessError{Type: engine.ErrStep, Cause: &PanicError{Value: "nil map"}}, Category: domain.CategoryStep}, record.ErrorInternal},
		{"lookup", Failure{Err: &engine.ProcessError{Type: engine.ErrStep, Cause: lookup}, Category: domain.CategoryDependencyTransient}, record.ErrorLookupUnavailable},
		{"dependency", Failure{Err: fmt.Errorf("x: %w", registry), Category: domain.CategoryDependencyTransient}, record.ErrorDependencyUnavailable},
		{"sandbox internal", Failure{Err: &engine.ProcessError{Type: engine.ErrStep, Cause: &sandbox.Error{Kind: sandbox.ErrorInternal}}, Category: domain.CategoryStep}, record.ErrorSandboxCrash},
		{"sandbox host", Failure{Err: &sandbox.Error{Kind: sandbox.ErrorHost}, Category: domain.CategoryStep}, record.ErrorSandboxCrash},
		{"sandbox compile", Failure{Err: &sandbox.Error{Kind: sandbox.ErrorCompile}, Category: domain.CategoryStep}, record.ErrorConfig},
		{"user script", Failure{Err: &engine.ProcessError{Type: engine.ErrStep, Cause: &sandbox.Error{Kind: sandbox.ErrorRuntime}}, Category: domain.CategoryStep}, record.ErrorStep},
		{"compile", Failure{Err: &engine.CompileError{}, Category: domain.CategoryStep}, record.ErrorConfig},
		{"unknown branch", Failure{Err: &engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: compile.ErrUnknownBranch}, Category: domain.CategoryOutputInvalid}, record.ErrorEncode},
		{"too large", Failure{Err: &engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: compile.ErrOutputTooLarge}, Category: domain.CategoryOutputInvalid}, record.ErrorOutputInvalid},
		{"timeout", Failure{Err: &engine.ProcessError{Type: engine.ErrStepTimeout}, Category: domain.CategoryStepTimeout}, record.ErrorTimeout},
		{"decode", Failure{Err: &engine.ProcessError{Type: engine.ErrDecode}, Category: domain.CategoryDecode}, record.ErrorDecode},
		{"registry outage on encode", Failure{Err: &engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: context.DeadlineExceeded}, Category: domain.CategoryDestinationTransient}, record.ErrorDependencyUnavailable},
	}
	for _, c := range cases {
		if got := (DeadLetterTypes{}).Of(c.f); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
	if (&PanicError{Value: 3}).Error() != "panic: 3" {
		t.Fatal("panic message")
	}
}

func TestClassifierDependencyErrors(t *testing.T) {
	lookup := &engine.DependencyError{Kind: engine.DependencyLookup, Cause: errors.New("refused")}
	cases := []struct {
		err  error
		want domain.ErrorCategory
	}{
		{&engine.ProcessError{Type: engine.ErrStep, Cause: lookup}, domain.CategoryDependencyTransient},
		{&engine.ProcessError{Type: engine.ErrStepTimeout, Cause: lookup}, domain.CategoryDependencyTransient},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: lookup}, domain.CategoryDestinationTransient},
		{lookup, domain.CategoryDependencyTransient},
		{&engine.ProcessError{Type: engine.ErrStep, Cause: errors.New("miss")}, domain.CategoryStep},
	}
	for _, c := range cases {
		if got := (DefaultClassifier{}).Classify(c.err); got != c.want {
			t.Errorf("%v: got %s want %s", c.err, got, c.want)
		}
	}
	if !(TransientCauses{}).Match(lookup) {
		t.Fatal("dependency error not transient")
	}
	if (DispositionTable{}).For(domain.CategoryDependencyTransient) != domain.DispositionApplyPolicy {
		t.Fatal("dependency disposition")
	}
}
