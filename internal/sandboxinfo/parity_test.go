package sandboxinfo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/runtime/reconcile"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// goldenDir holds the sandbox golden cases (js-sandbox JS-42/JS-110).
const goldenDir = "../../pkg/sandbox/testdata/golden"

type parityCase struct {
	Entry   script.EntryKind    `json:"entry"`
	Code    string              `json:"code"`
	Now     time.Time           `json:"now"`
	Limits  sandbox.RouteLimits `json:"limits"`
	Formats []string            `json:"formats"`
	Events  []struct {
		Key       any               `json:"key"`
		Value     any               `json:"value"`
		Headers   map[string]string `json:"headers"`
		Topic     string            `json:"topic"`
		Partition int32             `json:"partition"`
		Offset    int64             `json:"offset"`
		Timestamp time.Time         `json:"timestamp"`
	} `json:"events"`
}

// parityOutcome is what must be identical in preview and worker (logs are preview-only by design).
type parityOutcome struct {
	Value any
	Kind  sandbox.ErrorKind
	Msg   string
}

// TestPreviewWorkerSandboxParity runs every JSON-format golden case through the preview service's
// and the worker's sandbox wiring (both via the engine script port) and requires identical results.
// Both run under the preview route id: Math.random seeds include the route id, and preview runs do
// not carry the route id yet. Typed (Avro/Protobuf) cases wait for preview decoding (live-preview LP-101).
func TestPreviewWorkerSandboxParity(t *testing.T) {
	ctx := context.Background()
	pv, err := preview.NewSandboxScripts(ctx, sandboxtest.Salts{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pv.Close(ctx) })
	wm, err := sandbox.NewManager(ctx, sandbox.DefaultConfig(), sandbox.ManagerDeps{Engines: engines.Registry(),
		Salts: sandboxtest.Salts{}, Binder: enginebind.Binder{Base: ctxhelpers.Set{}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wm.Close(ctx) })
	files, _ := filepath.Glob(filepath.Join(goldenDir, "*.json"))
	sort.Strings(files)
	compared := 0
	for _, file := range files {
		gc := readParityCase(t, file)
		if len(gc.Formats) > 0 {
			continue
		}
		compared++
		t.Run(filepath.Base(file), func(t *testing.T) {
			worker := reconcile.SandboxScripts{Manager: wm}.Runtime(sandbox.RouteScope{RouteID: string(preview.PreviewRouteID),
				VersionID: "v1", TeamID: "t1", Parallelism: 2, Mode: sandbox.ScopeWorker})
			want := runParity(t, pv.Runtime(preview.RunScope{TeamID: "t1", Mode: preview.RunPreview}), gc)
			got := runParity(t, worker, gc)
			if reflect.DeepEqual(want, make([]parityOutcome, len(want))) {
				t.Fatal("case produced no values or errors")
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("preview and worker differ:\npreview %+v\nworker  %+v", want, got)
			}
		})
	}
	if compared == 0 {
		t.Fatal("no golden cases compared")
	}
}

func readParityCase(t *testing.T, file string) parityCase {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var gc parityCase
	if err := json.Unmarshal(raw, &gc); err != nil {
		t.Fatal(err)
	}
	return gc
}

func runParity(t *testing.T, rt script.Runtime, gc parityCase) []parityOutcome {
	t.Helper()
	ctx := context.Background()
	lim := script.Limits{Timeout: time.Duration(gc.Limits.JSTimeoutMs) * time.Millisecond}
	mod, err := rt.Compile(ctx, script.Source{Code: gc.Code, Entry: gc.Entry, Origin: "/branches/0/transform/steps/0/code"}, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(ctx)
	out := make([]parityOutcome, 0, len(gc.Events))
	for _, e := range gc.Events {
		ev := engine.Event{Key: e.Key, Value: e.Value, Topic: e.Topic, Partition: e.Partition, Offset: e.Offset, Timestamp: e.Timestamp}
		for k, v := range e.Headers {
			ev.Headers = append(ev.Headers, engine.Header{Key: k, Value: []byte(v)})
		}
		res, err := mod.Call(ctx, script.Invocation{Event: ev.Snapshot(), Now: gc.Now})
		o := parityOutcome{Value: res.Value}
		if se, ok := sandbox.AsError(err); ok {
			o = parityOutcome{Kind: se.Kind, Msg: se.Message}
		}
		out = append(out, o)
	}
	return out
}
