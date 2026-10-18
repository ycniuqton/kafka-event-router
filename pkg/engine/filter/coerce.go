package filter

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// WarningCode identifies a coercion or evaluation warning shown in preview.
type WarningCode string

const (
	WarnStringToNumber WarningCode = "coerced_string_to_number"
	WarnStringToBool   WarningCode = "coerced_string_to_bool"
	WarnIncomparable   WarningCode = "incomparable_types"
	WarnNotADate       WarningCode = "not_a_date"
	WarnNonBooleanExpr WarningCode = "non_boolean_expr"
)

// CoercionWarning is attached to a node result when a value was coerced or incomparable.
type CoercionWarning struct {
	Code WarningCode
}

// CodeString returns the code, or "" for a nil warning.
func (w *CoercionWarning) CodeString() string {
	if w == nil {
		return ""
	}
	return string(w.Code)
}

// BoolText holds the accepted textual booleans.
type BoolText string

const (
	BoolTextTrue  BoolText = "true"
	BoolTextFalse BoolText = "false"
)

// CoercionRule documents one row of the fixed coercion table (also published in engineCatalog.json).
type CoercionRule struct {
	Left    string      `json:"left"`
	Right   string      `json:"right"`
	Rule    string      `json:"rule"`
	Warning WarningCode `json:"warning,omitempty"`
}

// Coercer implements the fixed, documented coercion table used by every operator.
type Coercer struct{}

// Table returns the documented coercion rules.
func (Coercer) Table() []CoercionRule {
	return []CoercionRule{
		{Left: "number", Right: "numeric string", Rule: "parse right as number"},
		{Left: "numeric string", Right: "number", Rule: "parse left as number", Warning: WarnStringToNumber},
		{Left: "boolean or \"true\"/\"false\"", Right: "boolean or \"true\"/\"false\"", Rule: "parse string as boolean", Warning: WarnStringToBool},
		{Left: "RFC 3339 string or epoch ms", Right: "date operator", Rule: "parse to time"},
		{Left: "missing", Right: "any operator except exists/not_exists/is_empty", Rule: "false"},
		{Left: "null", Right: "is_empty", Rule: "true"},
		{Left: "other mismatched types", Right: "any", Rule: "false", Warning: WarnIncomparable},
	}
}

// Number returns the float value of numeric kinds (not strings).
func (Coercer) Number(v any) (float64, bool) {
	switch t := v.(type) {
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case float32:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

// NumericString parses a string holding a number.
func (Coercer) NumericString(v any) (float64, bool) {
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil
}

// Bool parses booleans and the strings "true"/"false"; coerced reports a string was parsed.
func (Coercer) Bool(v any) (value bool, ok bool, coerced bool) {
	switch t := v.(type) {
	case bool:
		return t, true, false
	case string:
		switch BoolText(t) {
		case BoolTextTrue:
			return true, true, true
		case BoolTextFalse:
			return false, true, true
		}
	}
	return false, false, false
}

// Numbers coerces a pair to numbers per the table; ok is false when either side is not numeric.
func (c Coercer) Numbers(left, right any) (l, r float64, w *CoercionWarning, ok bool) {
	l, lnum := c.Number(left)
	r, rnum := c.Number(right)
	switch {
	case lnum && rnum:
		return l, r, nil, true
	case lnum:
		r, ok = c.NumericString(right)
		return l, r, nil, ok
	case rnum:
		l, ok = c.NumericString(left)
		return l, r, &CoercionWarning{Code: WarnStringToNumber}, ok
	}
	return 0, 0, nil, false
}

// Equal compares with coercion; mismatched kinds are unequal with an incomparable warning.
func (c Coercer) Equal(left, right any) (bool, *CoercionWarning) {
	if l, r, w, ok := c.Numbers(left, right); ok {
		return c.intsEqual(left, right, l == r), w
	}
	if lb, lok, lc := c.Bool(left); lok {
		if rb, rok, rc := c.Bool(right); rok {
			return lb == rb, c.boolWarning(lc || rc, lc && rc)
		}
	}
	if ls, ok := left.(string); ok {
		if rs, ok := right.(string); ok {
			return ls == rs, nil
		}
	}
	if reflect.TypeOf(left) != reflect.TypeOf(right) {
		return false, &CoercionWarning{Code: WarnIncomparable}
	}
	return reflect.DeepEqual(left, right), nil
}

// intsEqual compares two int64 exactly, falling back to the float result.
func (Coercer) intsEqual(left, right any, floatEq bool) bool {
	li, lok := left.(int64)
	ri, rok := right.(int64)
	if lok && rok {
		return li == ri
	}
	return floatEq
}

// boolWarning warns when exactly one side was a textual boolean.
func (Coercer) boolWarning(anyCoerced, bothText bool) *CoercionWarning {
	if anyCoerced && !bothText {
		return &CoercionWarning{Code: WarnStringToBool}
	}
	return nil
}

// Compare orders numbers (with coercion) or two strings; ok is false when incomparable.
func (c Coercer) Compare(left, right any) (int, *CoercionWarning, bool) {
	if l, r, w, ok := c.Numbers(left, right); ok {
		return c.order(l < r, l > r), w, true
	}
	ls, lok := left.(string)
	rs, rok := right.(string)
	if lok && rok {
		return strings.Compare(ls, rs), nil, true
	}
	return 0, &CoercionWarning{Code: WarnIncomparable}, false
}

func (Coercer) order(less, greater bool) int {
	switch {
	case less:
		return -1
	case greater:
		return 1
	}
	return 0
}

// Time parses RFC 3339 strings and epoch milliseconds.
func (c Coercer) Time(v any) (time.Time, bool) {
	if s, ok := v.(string); ok {
		t, err := time.Parse(time.RFC3339Nano, s)
		return t, err == nil
	}
	if f, ok := c.Number(v); ok {
		return time.UnixMilli(int64(f)).UTC(), true
	}
	return time.Time{}, false
}
