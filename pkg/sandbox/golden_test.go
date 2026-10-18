package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// goldenCase is one file in testdata/golden: code, fixed events, expected results.
type goldenCase struct {
	Name   string              `json:"name"`
	Entry  sandbox.EntryKind   `json:"entry"`
	Code   string              `json:"code"`
	Now    time.Time           `json:"now"`
	Events []goldenEvent       `json:"events"`
	Expect []json.RawMessage   `json:"expect"`
	Limits sandbox.RouteLimits `json:"limits,omitempty"`
	// Formats marks schema-typed cases: event values are written tagged and decoded to the Go
	// values an Avro/Protobuf codec produces (int64, []byte, *big.Rat, time.Time).
	Formats []boundary.Format `json:"formats,omitempty"`
}

type goldenEvent struct {
	Key       any               `json:"key"`
	Value     any               `json:"value"`
	Headers   map[string]string `json:"headers"`
	Topic     string            `json:"topic"`
	Partition int32             `json:"partition"`
	Offset    int64             `json:"offset"`
	Timestamp time.Time         `json:"timestamp"`
}

func (e goldenEvent) guest(t *testing.T, typed bool) boundary.GuestEvent {
	ev := boundary.GuestEvent{Key: e.Key, Value: schemaValue(t, typed, e.Value), Topic: e.Topic, Partition: e.Partition, Offset: e.Offset, Timestamp: e.Timestamp}
	for k, v := range e.Headers {
		ev.Headers = append(ev.Headers, boundary.Header{Key: k, Value: []byte(v)})
	}
	return ev
}

// schemaValue decodes a tagged JSON value into codec-native Go values for typed cases.
func schemaValue(t *testing.T, typed bool, v any) any {
	if !typed {
		return v
	}
	raw, _ := json.Marshal(v)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	out, err := boundary.TaggedDecoder{Tags: boundary.StandardTags}.Decode(tree)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// goldenResult is the comparable form of a call outcome.
type goldenResult struct {
	Outputs []goldenOutput    `json:"outputs,omitempty"`
	Dropped bool              `json:"dropped,omitempty"`
	Bool    *bool             `json:"bool,omitempty"`
	Logs    []string          `json:"logs,omitempty"`
	Error   sandbox.ErrorKind `json:"error,omitempty"`
	Message string            `json:"message,omitempty"`
	Line    int               `json:"line,omitempty"`
}

type goldenOutput struct {
	Key     any               `json:"key,omitempty"`
	Value   any               `json:"value"`
	Headers map[string]string `json:"headers,omitempty"`
}

func toGolden(entry sandbox.EntryKind, typed bool, res sandbox.Result, err error) goldenResult {
	g := goldenResult{Dropped: res.Dropped, Logs: res.Logs}
	if se, ok := sandbox.AsError(err); ok {
		return goldenResult{Error: se.Kind, Message: se.Message, Line: se.Line, Logs: res.Logs}
	}
	if entry == sandbox.EntryPredicate {
		b := res.Bool
		g.Bool = &b
	}
	for _, o := range res.Outputs {
		out := goldenOutput{Key: o.Key, Value: o.Value}
		if typed {
			out.Value = boundary.TaggedEncoder{}.Encode(o.Value)
		}
		for _, h := range o.Headers {
			if out.Headers == nil {
				out.Headers = map[string]string{}
			}
			out.Headers[h.Key] = string(h.Value)
		}
		g.Outputs = append(g.Outputs, out)
	}
	return g
}

func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*.json")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	m := sandboxtest.NewManager(t)
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) { runGolden(t, m, file) })
	}
}

func runGolden(t *testing.T, m sandbox.Manager, file string) {
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var gc goldenCase
	if err := json.Unmarshal(raw, &gc); err != nil {
		t.Fatal(err)
	}
	scope := sandboxtest.Scope(1)
	scope.Formats = gc.Formats
	typed := len(gc.Formats) > 0
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: gc.Entry, Code: gc.Code}}, gc.Limits)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	var got []json.RawMessage
	for _, ev := range gc.Events {
		res, err := mod.Call(context.Background(), sandbox.Invocation{Event: ev.guest(t, typed), Now: gc.Now})
		b, _ := json.Marshal(toGolden(gc.Entry, typed, res, err))
		got = append(got, b)
	}
	if os.Getenv("ROUTER_UPDATE_GOLDEN") != "" {
		gc.Expect = got
		out, _ := json.MarshalIndent(gc, "", "  ")
		_ = os.WriteFile(file, append(out, '\n'), 0o644)
		return
	}
	compareGolden(t, gc.Expect, got)
}

func compareGolden(t *testing.T, want, got []json.RawMessage) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("expected %d results, got %d", len(want), len(got))
	}
	for i := range want {
		var w, g any
		_ = json.Unmarshal(want[i], &w)
		_ = json.Unmarshal(got[i], &g)
		if !reflect.DeepEqual(w, g) {
			t.Errorf("event %d:\n got  %s\n want %s", i, got[i], want[i])
		}
	}
}
