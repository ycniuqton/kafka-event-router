package preview_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

var update = flag.Bool("update", false, "rewrite golden RunResult files")

var goldenStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newRunner(t testing.TB) preview.PreviewRunner {
	t.Helper()
	b, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), preview.NoScripts{}, ctxhelpers.StaticSaltProvider{Value: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	return preview.PreviewRunner{Builder: b, Differ: preview.StructuralDiffer{}, Deadline: preview.RunDeadline.Duration(), Now: func() time.Time { return goldenStart }}
}

func readRecords(t *testing.T, path string) []preview.SampleRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs := []preview.SampleRecord{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		i := len(recs)
		recs = append(recs, preview.SampleRecord{Seq: i, Topic: "in", Offset: int64(i), Timestamp: goldenStart.Add(time.Duration(i) * time.Second), Value: append([]byte(nil), line...)})
	}
	return recs
}

// stripTimings zeroes wall-clock fields so goldens are stable.
func stripTimings(res *preview.RunResultDTO) {
	res.Summary.DurationMs = 0
	for i := range res.Results {
		for j := range res.Results[i].Branches {
			for k := range res.Results[i].Branches[j].Steps {
				res.Results[i].Branches[j].Steps[k].DurationUs = 0
			}
		}
	}
}

// TestGoldenRunResults runs every testdata/golden case (route.json + input.jsonl → expected.json).
func TestGoldenRunResults(t *testing.T) {
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
			res, err := newRunner(t).Run(context.Background(), preview.RunPlan{Config: raw, Records: recs, Loaded: len(recs), Scope: preview.RunScope{TeamID: "t1", Mode: preview.RunPreview}})
			if err != nil {
				t.Fatal(err)
			}
			stripTimings(&res)
			got, _ := json.MarshalIndent(res, "", " ")
			path := filepath.Join(dir, "expected.json")
			if *update {
				if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden (run with -update): %v", err)
			}
			if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
				t.Errorf("result differs from %s:\n%s", path, got)
			}
		})
	}
}

func TestRunClassifiesStatusesAndSummary(t *testing.T) {
	raw, _ := os.ReadFile("testdata/golden/filter_only/route.json")
	recs := readRecords(t, "testdata/golden/filter_only/input.jsonl")
	res, err := newRunner(t).Run(context.Background(), preview.RunPlan{Config: raw, Records: recs, Loaded: len(recs)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Passed == 0 || res.Summary.Filtered == 0 || res.Summary.Processed != len(recs) {
		t.Fatalf("summary = %+v", res.Summary)
	}
	for _, r := range res.Results {
		b := r.Branches[0]
		if r.Status == preview.StatusFiltered && len(b.Filter) == 0 {
			t.Errorf("filtered event %d has no node results", r.Seq)
		}
		if r.Status == preview.StatusPassed && (len(b.Outputs) != 1 || !b.Outputs[0].Valid || b.Outputs[0].Topic != "orders-paid") {
			t.Errorf("passed event %d outputs = %+v", r.Seq, b.Outputs)
		}
		for _, n := range b.Filter {
			if len(n.Path) < len("/branches/0/filter") || n.Path[:len("/branches/0/filter")] != "/branches/0/filter" {
				t.Errorf("node path %q is not absolute", n.Path)
			}
		}
	}
}

func TestRunStepPatchesReplayToOutput(t *testing.T) {
	raw, _ := os.ReadFile("testdata/golden/mask_rename/route.json")
	recs := readRecords(t, "testdata/golden/mask_rename/input.jsonl")
	res, err := newRunner(t).Run(context.Background(), preview.RunPlan{Config: raw, Records: recs[:1], Loaded: 1})
	if err != nil {
		t.Fatal(err)
	}
	b := res.Results[0].Branches[0]
	var input any
	_ = json.Unmarshal(recs[0].Value, &input)
	doc := any(map[string]any{"value": input})
	for _, s := range b.Steps {
		doc, err = preview.PatchApplier{}.Apply(doc, s.Patch)
		if err != nil {
			t.Fatalf("step %d: %v", s.Index, err)
		}
	}
	got, _ := json.Marshal(doc.(map[string]any)["value"])
	want, _ := json.Marshal(b.Outputs[0].Value)
	if !bytes.Equal(got, want) {
		t.Fatalf("replayed %s, output %s", got, want)
	}
}

func TestRunInvalidConfigIs422WithPaths(t *testing.T) {
	_, err := newRunner(t).Run(context.Background(), preview.RunPlan{Config: []byte(`{"apiVersion":"router/v1","name":"x","source":{},"branches":[]}`)})
	ice, ok := err.(*preview.InvalidConfigError)
	if !ok || len(ice.FieldErrors()) == 0 || ice.ProblemPath()[:len("/config")] != "/config" {
		t.Fatalf("err = %#v", err)
	}
}

func TestRunDeadlineMarksIncomplete(t *testing.T) {
	raw, _ := os.ReadFile("testdata/golden/filter_only/route.json")
	recs := readRecords(t, "testdata/golden/filter_only/input.jsonl")
	r := newRunner(t)
	r.Deadline = time.Nanosecond
	res, err := r.Run(context.Background(), preview.RunPlan{Config: raw, Records: recs, Loaded: len(recs)})
	if err == nil && !res.Summary.Incomplete {
		t.Fatalf("expected incomplete run, got %+v", res.Summary)
	}
}

func TestRunJSStepInSandboxWithLogs(t *testing.T) {
	scripts, err := preview.NewSandboxScripts(context.Background(), preview.NewProcessSaltProvider())
	if err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	defer scripts.Close(context.Background())
	b, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), scripts, preview.NewProcessSaltProvider())
	if err != nil {
		t.Fatal(err)
	}
	r := preview.PreviewRunner{Builder: b, Differ: preview.StructuralDiffer{}, Deadline: preview.RunDeadline.Duration(), Now: time.Now}
	route := `{"apiVersion":"router/v1","name":"js","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[{"id":"j","kind":"js","code":"function transform(event, ctx) { ctx.log('hi'); return { value: Object.assign({}, event.value, { b: 2 }) }; }"}]},
"destination":{"connectionId":"c","topic":"out"}}]}`
	recs := []preview.SampleRecord{{Seq: 0, Value: []byte(`{"a":1}`), Timestamp: goldenStart}}
	res, err := r.Run(context.Background(), preview.RunPlan{Config: []byte(route), Records: recs, Loaded: 1, Scope: preview.RunScope{TeamID: "t", Mode: preview.RunPreview}})
	if err != nil {
		t.Fatal(err)
	}
	ev := res.Results[0]
	if ev.Status != preview.StatusPassed || len(ev.Branches[0].Steps) != 1 {
		t.Fatalf("result = %+v", ev)
	}
	step := ev.Branches[0].Steps[0]
	if len(step.Logs) != 1 || len(step.Patch) != 1 || step.Patch[0].Op != "add" {
		t.Fatalf("step = %+v outputs %s", step, mustJSON(ev.Branches[0].Outputs))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
