package escapetest

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/eventrouter/router/pkg/sandbox"
)

// Expectation verifies the outcome of one call.
type Expectation interface {
	Verify(res sandbox.Result, err error) error
}

// FailsWith expects a typed sandbox error of one of Kinds.
type FailsWith struct {
	Kinds []sandbox.ErrorKind
}

func (f FailsWith) Verify(_ sandbox.Result, err error) error {
	se, ok := sandbox.AsError(err)
	if !ok || !slices.Contains(f.Kinds, se.Kind) {
		return fmt.Errorf("want error kind %v, got %v", f.Kinds, err)
	}
	return nil
}

// Returns expects success with a value deeply equal to Value.
type Returns struct {
	Value any
}

func (r Returns) Verify(res sandbox.Result, err error) error {
	if err != nil {
		return fmt.Errorf("want value %v, got error %v", r.Value, err)
	}
	if len(res.Outputs) != 1 || !reflect.DeepEqual(res.Outputs[0].Value, r.Value) {
		return fmt.Errorf("want value %v, got %+v", r.Value, res.Outputs)
	}
	return nil
}
