package formula

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// ErrNotNumber is the cause when an operand cannot be read as a number.
var ErrNotNumber = errors.New("value is not a number")

// ErrNotDate is the cause when a value is neither an RFC 3339 string nor epoch millis.
var ErrNotDate = errors.New("value is not a date")

// ErrDivisionByZero is the cause of `/` and `%` with a zero divisor.
var ErrDivisionByZero = errors.New("division by zero")

// EvalError is a runtime failure inside a formula function.
type EvalError struct {
	Func  FuncName
	Cause error
}

func (e *EvalError) Error() string { return fmt.Sprintf("formula %s(): %v", e.Func, e.Cause) }

func (e *EvalError) Unwrap() error { return e.Cause }

// Numbers reads values as numbers with the engine coercion table.
type Numbers struct{}

// Of returns v as float64; numeric strings are parsed, null is not a number.
func (Numbers) Of(v any) (float64, error) {
	if f, ok := (filter.Coercer{}).Number(v); ok {
		return f, nil
	}
	if f, ok := (filter.Coercer{}).NumericString(v); ok {
		return f, nil
	}
	return 0, fmt.Errorf("%w: %T", ErrNotNumber, v)
}

// IsInteger reports whether v is an engine integer.
func (Numbers) IsInteger(v any) bool {
	_, ok := v.(int64)
	return ok
}

// Result returns an int64 when integral is requested and f is a safe whole number.
func (Numbers) Result(f float64, integral bool) any {
	if integral && f == math.Trunc(f) && math.Abs(f) < math.MaxInt64 {
		return int64(f)
	}
	return f
}

// Truth is formula truthiness: false, null, 0 and "" are false, everything else true.
type Truth struct{}

// Of returns the truth value of v.
func (Truth) Of(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	}
	if f, ok := (filter.Coercer{}).Number(v); ok {
		return f != 0
	}
	return true
}

// Text renders values as strings: null is empty, scalars as text, containers as JSON.
type Text struct{}

// Of returns the textual form of v.
func (Text) Of(v any) string {
	if b, ok := (codec.ScalarText{}).Of(v); ok {
		return string(b)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
