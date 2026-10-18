package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/batch"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/trace"
)

var update = flag.Bool("update", false, "rewrite golden expected files")

// goldenFile names the files of one golden case.
type goldenFile string

const (
	fileRoute          goldenFile = "route.json"
	fileInput          goldenFile = "input.jsonl"
	fileExpected       goldenFile = "expected.jsonl"
	fileExpectedErrors goldenFile = "expected_errors.jsonl"
	fileExpectedTrace  goldenFile = "expected_trace.jsonl"
	fileRegistry       goldenFile = "registry.json"
	fileLibrary        goldenFile = "library"
)

func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) < 10 {
		t.Fatalf("want >= 10 golden cases, got %d (%v)", len(dirs), err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) { goldenCase{dir: dir}.run(t) })
	}
}

type goldenCase struct {
	dir string
}

func (g goldenCase) path(f goldenFile) string { return filepath.Join(g.dir, string(f)) }

func (g goldenCase) run(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile(g.path(fileRoute))
	if err != nil {
		t.Fatal(err)
	}
	opts := enginetest.GoldenOptions()
	wire := g.wire(t, &opts)
	g.library(t, &opts)
	g.validate(t, raw, opts)
	doc, _ := config.Parse(raw)
	p, err := compile.NewDefaultCompiler().Compile(ctx, doc, opts)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer p.Close(ctx)
	in, err := os.Open(g.path(fileInput))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var outs, errs, traces bytes.Buffer
	sinks := batch.Sinks{Outputs: &outs, Errors: &errs}
	_, traceWanted := os.Stat(g.path(fileExpectedTrace))
	if traceWanted == nil {
		sinks.Traces = &traces
	}
	cfg, _ := doc.Config()
	runner := batch.Runner{Pipeline: p, Topic: cfg.Source.Topics[0], Timestamp: enginetest.GoldenNow}
	if wire != nil {
		runner.Input, runner.Output = wire.Input(t, cfg), wire
	}
	if _, err := runner.Run(ctx, in, sinks); err != nil {
		t.Fatal(err)
	}
	g.compare(t, fileExpected, outs.Bytes())
	g.compare(t, fileExpectedErrors, errs.Bytes())
	if traceWanted == nil {
		g.compare(t, fileExpectedTrace, stableTraces(t, traces.Bytes()))
	}
}

// wire loads registry.json (schema-registry cases) into a fake registry used by the
// pipeline and by the JSON ↔ wire adapters of the batch runner.
func (g goldenCase) wire(t *testing.T, opts *engine.CompileOptions) *enginetest.WireAdapter {
	raw, err := os.ReadFile(g.path(fileRegistry))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	w, err := enginetest.NewWireAdapter(raw)
	if err != nil {
		t.Fatal(err)
	}
	opts.SchemaRegistries = w.Registry
	return w
}

// library loads the case's library directory (transforms and lookup tables) when present.
func (g goldenCase) library(t *testing.T, opts *engine.CompileOptions) {
	if _, err := os.Stat(g.path(fileLibrary)); os.IsNotExist(err) {
		return
	}
	lib, err := configtest.LibraryDir{Path: g.path(fileLibrary)}.Load()
	if err != nil {
		t.Fatal(err)
	}
	opts.Library = lib
}

func (g goldenCase) validate(t *testing.T, raw []byte, opts engine.CompileOptions) {
	m, err := validators.NewDefaultManager(validators.Deps{Compiler: compile.NewDefaultCompiler(), Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	issues, err := m.Validate(context.Background(), config.StageOffline, "", raw)
	if err != nil || issues.HasErrors() {
		t.Fatalf("golden route invalid: %v %+v", err, issues)
	}
}

func (g goldenCase) compare(t *testing.T, f goldenFile, got []byte) {
	t.Helper()
	if *update {
		if len(got) == 0 && f == fileExpectedErrors {
			_ = os.Remove(g.path(f))
			return
		}
		if err := os.WriteFile(g.path(f), got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(g.path(f))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s mismatch\n--- want\n%s\n--- got\n%s", f, want, got)
	}
}

// stableTraces zeroes step durations so traces are deterministic.
func stableTraces(t *testing.T, raw []byte) []byte {
	var out bytes.Buffer
	dec := json.NewDecoder(bytes.NewReader(raw))
	enc := json.NewEncoder(&out)
	for dec.More() {
		var tr trace.EventTrace
		if err := dec.Decode(&tr); err != nil {
			t.Fatal(err)
		}
		for i := range tr.Branches {
			for j := range tr.Branches[i].Steps {
				tr.Branches[i].Steps[j].DurationUs = 0
			}
		}
		if err := enc.Encode(tr); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

// TestLibraryLookupParity checks that a lookup recipe on a pinned library table and the
// same lookup through ctx.lookup in JS produce identical events (PE-111).
func TestLibraryLookupParity(t *testing.T) {
	g := goldenCase{dir: filepath.Join("testdata", "golden", "library_lookup")}
	raw, err := os.ReadFile(g.path(fileExpected))
	if err != nil {
		t.Fatal(err)
	}
	byBranch := map[string][]json.RawMessage{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var o struct {
			Branch string          `json:"branch"`
			Value  json.RawMessage `json:"value"`
		}
		if err := dec.Decode(&o); err != nil {
			t.Fatal(err)
		}
		byBranch[o.Branch] = append(byBranch[o.Branch], o.Value)
	}
	recipe, js := byBranch["recipe"], byBranch["js"]
	if len(recipe) == 0 || len(recipe) != len(js) {
		t.Fatalf("outputs: recipe=%d js=%d", len(recipe), len(js))
	}
	for i := range recipe {
		if !bytes.Equal(recipe[i], js[i]) {
			t.Errorf("event %d differs:\nrecipe %s\njs     %s", i, recipe[i], js[i])
		}
	}
}
