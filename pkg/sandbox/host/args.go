package host

import (
	"encoding/json"
	"fmt"
	"time"
)

// Args reads typed positional arguments of one helper call.
type Args struct {
	name FnName
	raw  []json.RawMessage
}

// ArgsOf wraps the raw arguments of call for helper name.
func ArgsOf(name FnName, call *Call) Args {
	return Args{name: name, raw: call.Args}
}

// Require fails unless at least n arguments were passed.
func (a Args) Require(n int) error {
	if len(a.raw) < n {
		return fmt.Errorf(string(MsgArgCount), a.name, n, len(a.raw))
	}
	return nil
}

// Value decodes argument i as a generic JSON value (missing → nil).
func (a Args) Value(i int) (any, error) {
	if i >= len(a.raw) {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(a.raw[i], &v); err != nil {
		return nil, fmt.Errorf(string(MsgBadArguments), a.name, err)
	}
	return v, nil
}

// String decodes argument i as a string; a missing optional argument yields "".
func (a Args) String(i int) (string, error) {
	v, err := a.Value(i)
	if err != nil || v == nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf(string(MsgNotString), a.name, i+1)
	}
	return s, nil
}

// Time decodes argument i as an ISO-8601 string or epoch milliseconds.
func (a Args) Time(i int) (time.Time, error) {
	v, err := a.Value(i)
	if err != nil {
		return time.Time{}, err
	}
	return TimeValue{}.Parse(a.name, v)
}

// TimeValue converts JS date representations into time.Time.
type TimeValue struct{}

// Parse accepts RFC 3339 strings and numbers of epoch milliseconds.
func (TimeValue) Parse(name FnName, v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		return time.Parse(time.RFC3339Nano, t)
	case float64:
		return time.UnixMilli(int64(t)).UTC(), nil
	}
	return time.Time{}, fmt.Errorf(string(MsgBadArguments), name, v)
}

// TimeLayout is a Go time layout used at the JS boundary.
type TimeLayout string

// ISOLayout matches JavaScript Date#toISOString (UTC, milliseconds).
const ISOLayout TimeLayout = "2006-01-02T15:04:05.000Z"

// ISO renders t the way ctx.now(), event.timestamp and Date#toISOString do.
func (TimeValue) ISO(t time.Time) string {
	return t.UTC().Format(string(ISOLayout))
}
