package reconcile

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

// failingLibrary returns err for every load.
type failingLibrary struct{ err error }

func (f failingLibrary) Transform(context.Context, string, int) (config.Steps, error) {
	return nil, f.err
}

func (f failingLibrary) LookupTable(context.Context, string, int) (config.LookupRows, error) {
	return nil, f.err
}

func TestLibraryProbeClassifies(t *testing.T) {
	compileErr := errors.New("compile failed")
	missing := NewLibraryProbe(failingLibrary{err: fmt.Errorf("%w: x@1", config.ErrLibraryRefMissing)})
	_, _ = missing.Resolver().Transform(context.Background(), "x", 1)
	var ce *CompileError
	if !errors.As(missing.Classify(compileErr), &ce) {
		t.Fatal("missing pin must stay a deterministic CompileError")
	}
	down := NewLibraryProbe(failingLibrary{err: errors.New("control plane down")})
	_, _ = down.Resolver().LookupTable(context.Background(), "t", 1)
	if err := down.Classify(compileErr); errors.As(err, &ce) || (CompileFailures{}).Match(err) {
		t.Fatalf("transient load must be retryable, got %v", err)
	}
	if NewLibraryProbe(nil).Resolver() != nil {
		t.Fatal("nil library must compile without a resolver")
	}
}
