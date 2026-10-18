package preview_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// parityTeam is the team both sides hash with (same salt provider as newRunner).
const parityTeam = "t1"

// workerProcessor compiles raw the way the worker's EngineVersionCompiler does and wraps it in the
// worker's RecordProcessor (decode → Process → encode per branch).
func workerProcessor(t *testing.T, raw []byte) *runner.RecordProcessor {
	t.Helper()
	doc, err := config.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		t.Fatal(err)
	}
	salts := ctxhelpers.StaticSaltProvider{Value: []byte("salt")}
	opts := engine.CompileOptions{Helpers: ctxhelpers.Set{Hash: ctxhelpers.NewSaltedHasher(salts, parityTeam)}.WithDefaults(), Meta: engine.RouteMeta{RouteID: "r1", TeamID: parityTeam}}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	return runner.NewRecordProcessor(runner.CompiledVersion{Config: cfg, Pipeline: p})
}

// parityOutcome is what both sides decided for one event: failed, or these outputs (none when filtered/dropped).
type parityOutcome struct {
	Failed  bool
	Outputs string
}

// workerOutcome renders the worker's outgoing records like preview OutputDTOs (key, value, headers).
func workerOutcome(t *testing.T, proc *runner.RecordProcessor, rec preview.SampleRecord) parityOutcome {
	t.Helper()
	outs, err := proc.Process(context.Background(), rec.Raw())
	if err != nil {
		return parityOutcome{Failed: true}
	}
	rendered := make([]map[string]any, 0, len(outs))
	for _, o := range outs {
		r := preview.BytesRenderer{}
		rendered = append(rendered, map[string]any{"topic": o.Record.Topic, "key": r.Value(o.Record.Key), "value": r.Value(o.Record.Value), "headers": r.Headers(o.Record.Headers)})
	}
	return parityOutcome{Outputs: canonical(t, rendered)}
}

// previewOutcome renders the outputs of one preview result the same way.
func previewOutcome(t *testing.T, res preview.EventResultDTO) parityOutcome {
	t.Helper()
	if res.Status == preview.StatusError {
		return parityOutcome{Failed: true}
	}
	rendered := []map[string]any{}
	for _, b := range res.Branches {
		for _, o := range b.Outputs {
			rendered = append(rendered, map[string]any{"topic": o.Topic, "key": o.Key, "value": o.Value, "headers": o.Headers})
		}
	}
	return parityOutcome{Outputs: canonical(t, rendered)}
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(generic)
	return string(out)
}

// TestPreviewWorkerParity (NFR-1, LP-62): the same config and samples give the same outputs and the same
// pass/no-output decision in the preview runner and the worker's record processor (every golden case).
func TestPreviewWorkerParity(t *testing.T) {
	dirs, _ := filepath.Glob("testdata/golden/*")
	if len(dirs) == 0 {
		t.Fatal("no golden cases")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, "route.json"))
			if err != nil {
				t.Fatal(err)
			}
			recs := readRecords(t, filepath.Join(dir, "input.jsonl"))
			res, err := newRunner(t).Run(context.Background(), preview.RunPlan{Config: raw, Records: recs, Loaded: len(recs), Scope: preview.RunScope{TeamID: parityTeam, Mode: preview.RunPreview}})
			if err != nil {
				t.Fatal(err)
			}
			proc := workerProcessor(t, raw)
			for i, rec := range recs {
				want, got := previewOutcome(t, res.Results[i]), workerOutcome(t, proc, rec)
				if want != got {
					t.Errorf("event %d differs:\npreview: %+v\nworker:  %+v", i, want, got)
				}
			}
		})
	}
}
