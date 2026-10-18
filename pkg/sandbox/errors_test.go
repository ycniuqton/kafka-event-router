package sandbox

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorClassifierRetryableOnlyForTimeout(t *testing.T) {
	c := NewErrorClassifier()
	discard := map[ErrorKind]bool{ErrorTimeout: true, ErrorMemory: true, ErrorStackOverflow: true, ErrorInternal: true}
	kinds := []ErrorKind{ErrorCompile, ErrorRuntime, ErrorTimeout, ErrorMemory, ErrorStackOverflow,
		ErrorOutputTooLarge, ErrorInvalidOutput, ErrorHost, ErrorInternal}
	for _, k := range kinds {
		p := c.Policy(k)
		if p.Retryable != (k == ErrorTimeout) {
			t.Errorf("%s retryable = %v", k, p.Retryable)
		}
		if (p.Action == PoolDiscard) != discard[k] {
			t.Errorf("%s action = %s", k, p.Action)
		}
	}
	if c.Policy("bogus") != c.Policy(ErrorInternal) {
		t.Error("unknown kinds must be treated as internal")
	}
}

func TestErrorClassifierClassify(t *testing.T) {
	c := NewErrorClassifier()
	wrapped := fmt.Errorf("step: %w", c.New(ErrorTimeout, "slow"))
	if got := c.Classify(wrapped); got.Kind != ErrorTimeout || !got.Retryable {
		t.Fatalf("wrapped timeout = %+v", got)
	}
	if got := c.Classify(errors.New("boom")); got.Kind != ErrorInternal || got.Retryable {
		t.Fatalf("plain error = %+v", got)
	}
	exhausted := &Error{Kind: ErrorInternal, Message: string(MsgPoolExhausted), Retryable: true}
	if got := c.Classify(exhausted); !got.Retryable {
		t.Fatal("pool exhaustion must stay retryable (backpressure)")
	}
}

func TestErrorFormatting(t *testing.T) {
	e := &Error{Kind: ErrorRuntime, Message: "TypeError: x", Line: 3, Column: 7}
	if e.Error() != "runtime: TypeError: x (3:7)" {
		t.Fatalf("Error() = %q", e.Error())
	}
	d := DiagnosticFromError("/branches/0/transform/steps/1/code", e)
	if d.Severity != SeverityError || d.Line != 3 || d.Column != 7 || d.Path == "" {
		t.Fatalf("diagnostic = %+v", d)
	}
}
