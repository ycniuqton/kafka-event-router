package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestDependencyErrorContract(t *testing.T) {
	de := &DependencyError{Kind: DependencyLookup, Name: "customers", Cause: context.DeadlineExceeded}
	err := &ProcessError{Type: ErrStep, StepID: "lk", Cause: fmt.Errorf("lookup step: %w", de)}
	got, ok := AsDependencyError(err)
	if !ok || got.Kind != DependencyLookup || !got.Transient() {
		t.Fatalf("got %+v %v", got, ok)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cause not unwrapped")
	}
	if _, ok := AsDependencyError(errors.New("miss")); ok {
		t.Fatal("plain error matched")
	}
	if (&DependencyError{Kind: DependencyHTTP, Cause: errors.New("503")}).Error() != "http unavailable: 503" {
		t.Fatal("message")
	}
}
