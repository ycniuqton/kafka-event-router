package host

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeServices struct{ calls []string }

func (f *fakeServices) Masker() Masker               { return f }
func (f *fakeServices) Hasher() Hasher               { return f }
func (f *fakeServices) DateFormatter() DateFormatter { return f }
func (f *fakeServices) Lookups() LookupResolver      { return f }

func (f *fakeServices) Mask(v any, s MaskStyle) (any, error) {
	f.calls = append(f.calls, "mask:"+string(s))
	return "masked", nil
}
func (f *fakeServices) Hash(_ context.Context, v any) (string, error) {
	f.calls = append(f.calls, "hash")
	return "h", nil
}
func (f *fakeServices) Format(t time.Time, layout, tz string) (string, error) {
	f.calls = append(f.calls, "date:"+t.Format(time.RFC3339)+"|"+layout+"|"+tz)
	return "d", nil
}
func (f *fakeServices) Lookup(table, key string) (any, bool) {
	f.calls = append(f.calls, "lookup:"+table+"/"+key)
	if key == "missing" {
		return nil, false
	}
	return "v", true
}

func dispatch(t *testing.T, r *Registry, name FnName, args string) (string, error) {
	t.Helper()
	out, err := r.Dispatch(name, []byte(args), &Call{Ctx: context.Background(), Now: time.Unix(60, 0), Logs: NewLogBuffer(100)})
	return string(out), err
}

func TestRegistryDelegatesToServices(t *testing.T) {
	f := &fakeServices{}
	r := NewStandardRegistry(f)
	steps := []struct {
		fn   FnName
		args string
		want string
		call string
	}{
		{FnMaskEmail, `["a@b.c"]`, `"masked"`, "mask:email"},
		{FnMaskLast4, `["1234567"]`, `"masked"`, "mask:last4"},
		{FnMaskFull, `["x"]`, `"masked"`, "mask:full"},
		{FnHash, `["x"]`, `"h"`, "hash"},
		{FnFormatDate, `["2024-01-02T03:04:05Z","date","Europe/Berlin"]`, `"d"`, "date:2024-01-02T03:04:05Z|date|Europe/Berlin"},
		{FnFormatDate, `[0,"rfc3339"]`, `"d"`, "date:1970-01-01T00:00:00Z|rfc3339|"},
		{FnLookup, `["countries","DE"]`, `"v"`, "lookup:countries/DE"},
		{FnLookup, `["countries",42]`, `"v"`, "lookup:countries/42"},
		{FnLookup, `["countries","missing"]`, `null`, "lookup:countries/missing"},
	}
	for i, s := range steps {
		got, err := dispatch(t, r, s.fn, s.args)
		if err != nil || got != s.want || f.calls[i] != s.call {
			t.Errorf("%s %s = %s, %v (call %q)", s.fn, s.args, got, err, f.calls[i])
		}
	}
}

func TestRegistryNowAndLog(t *testing.T) {
	r := NewStandardRegistry(&fakeServices{})
	call := &Call{Now: time.Unix(60, 5e6), Logs: NewLogBuffer(100)}
	out, err := r.Dispatch(FnNow, []byte(`[]`), call)
	if err != nil || string(out) != `"1970-01-01T00:01:00.005Z"` {
		t.Fatalf("now = %s, %v", out, err)
	}
	if _, err := r.Dispatch(FnLog, []byte(`["hello"]`), call); err != nil || call.Logs.Lines()[0] != "hello" {
		t.Fatalf("log = %v %q", err, call.Logs.Lines())
	}
}

func TestRegistryErrors(t *testing.T) {
	r := NewStandardRegistry(&fakeServices{})
	for _, tc := range []struct {
		fn   FnName
		args string
	}{
		{"fs.read", `[]`},
		{FnHash, `not json`},
		{FnFormatDate, `["2024-01-01T00:00:00Z"]`},
		{FnFormatDate, `[true,"date"]`},
		{FnLookup, `[1,"x"]`},
	} {
		if _, err := dispatch(t, r, tc.fn, tc.args); err == nil {
			t.Errorf("%s %s: expected error", tc.fn, tc.args)
		}
	}
	failing := &failingServices{}
	if _, err := NewStandardRegistry(failing).Dispatch(FnHash, []byte(`["x"]`), &Call{}); !errors.Is(err, errHashDown) {
		t.Fatalf("service errors must propagate, got %v", err)
	}
}

var errHashDown = errors.New("hash down")

type failingServices struct{ fakeServices }

func (f *failingServices) Hasher() Hasher { return f }
func (f *failingServices) Hash(context.Context, any) (string, error) {
	return "", errHashDown
}

func TestDocsCatalogue(t *testing.T) {
	docs := NewStandardRegistry(nil).Docs()
	raw, _ := json.Marshal(docs)
	if len(docs) != 8 || docs[0].Name != FnMaskEmail || docs[7].Name != FnLog {
		t.Fatalf("docs = %s", raw)
	}
}
