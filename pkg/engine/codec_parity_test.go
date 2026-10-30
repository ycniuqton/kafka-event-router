package engine_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// ParityBudget sizes the randomized part of the differential test.
type ParityBudget int

const (
	variantsPerLine  ParityBudget = 150
	minFastGoldens   ParityBudget = 12
	parityRouteSeeds ParityBudget = 400
)

// goldenRoots are the golden suites run through both codecs: engine and preview goldens.
var goldenRoots = []string{filepath.Join("testdata", "golden"), filepath.Join("..", "..", "internal", "preview", "testdata", "golden")}

// codecPair is one route compiled with the fast and with the tree codec.
type codecPair struct {
	fast, tree engine.RecordRunner
	topic      string
}

// newCodecPair compiles raw twice with opts.
func newCodecPair(t testing.TB, raw []byte, opts engine.CompileOptions) codecPair {
	t.Helper()
	doc, err := config.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := doc.Config()
	pair := codecPair{topic: "in"}
	if len(cfg.Source.Topics) > 0 {
		pair.topic = cfg.Source.Topics[0]
	}
	for _, mode := range []engine.CodecMode{engine.CodecFast, engine.CodecTree} {
		opts.Codec = mode
		p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, opts)
		if err != nil {
			t.Fatalf("compile %s: %v", mode, err)
		}
		t.Cleanup(func() { _ = p.Close(context.Background()) })
		runner, ok := p.(engine.RecordRunner)
		if !ok {
			t.Fatal("compiled pipeline does not implement engine.RecordRunner")
		}
		if mode == engine.CodecFast {
			pair.fast = runner
		} else {
			pair.tree = runner
		}
	}
	return pair
}

// check runs one value through both codecs and requires identical records and errors.
func (c codecPair) check(t testing.TB, value []byte, offset int64) {
	t.Helper()
	rec := engine.RawRecord{Key: []byte("k"), Value: value, Topic: c.topic, Offset: offset, Timestamp: enginetest.GoldenNow,
		Headers: []engine.Header{{Key: "h", Value: []byte("v")}}}
	gotFast, errFast := c.fast.Run(context.Background(), c.clone(rec))
	gotTree, errTree := c.tree.Run(context.Background(), c.clone(rec))
	if fmt.Sprint(errFast) != fmt.Sprint(errTree) || c.errType(errFast) != c.errType(errTree) {
		t.Fatalf("errors differ for %q:\nfast %v\ntree %v", value, errFast, errTree)
	}
	if !reflect.DeepEqual(c.normalize(gotFast), c.normalize(gotTree)) {
		t.Fatalf("outputs differ for %q (plan %s):\nfast %s\ntree %s", value, c.fast.CodecPlan(), c.render(gotFast), c.render(gotTree))
	}
}

func (codecPair) clone(r engine.RawRecord) engine.RawRecord {
	r.Value = bytes.Clone(r.Value)
	r.Headers = append([]engine.Header(nil), r.Headers...)
	return r
}

func (codecPair) errType(err error) engine.ErrorType {
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		return pe.Type
	}
	return ""
}

// normalize maps empty and nil byte slices alike (both encode the same Kafka record).
func (codecPair) normalize(es []engine.Encoded) []engine.Encoded {
	out := make([]engine.Encoded, 0, len(es))
	for _, e := range es {
		if len(e.Record.Headers) == 0 {
			e.Record.Headers = nil
		}
		out = append(out, e)
	}
	return out
}

func (codecPair) render(es []engine.Encoded) string {
	var b bytes.Buffer
	for _, e := range es {
		fmt.Fprintf(&b, "[%s key=%q value=%s headers=%v] ", e.Branch, e.Record.Key, e.Record.Value, e.Record.Headers)
	}
	return b.String()
}

// goldenParityCase loads one golden directory for the differential test.
type goldenParityCase struct {
	dir string
}

func (g goldenParityCase) run(t *testing.T) bool {
	raw, err := os.ReadFile(filepath.Join(g.dir, string(fileRoute)))
	if err != nil {
		t.Fatal(err)
	}
	opts := enginetest.GoldenOptions()
	gc := goldenCase{dir: g.dir}
	wire := gc.wire(t, &opts)
	gc.library(t, &opts)
	pair := newCodecPair(t, raw, opts)
	lines := g.lines(t, raw, wire)
	r := rand.New(rand.NewPCG(uint64(len(g.dir)), 7))
	mut := enginetest.EventMutator{R: r, Keys: g.keys(lines)}
	for i, line := range lines {
		pair.check(t, line, int64(i))
		if wire != nil {
			continue
		}
		for v := 0; v < int(variantsPerLine); v++ {
			pair.check(t, mut.Variant(line), int64(i))
		}
	}
	t.Logf("codec plan: %s", pair.fast.CodecPlan())
	return pair.fast.CodecPlan().Fast()
}

// lines returns the record values of input.jsonl (wire-encoded for registry cases).
func (g goldenParityCase) lines(t *testing.T, route []byte, wire *enginetest.WireAdapter) [][]byte {
	f, err := os.Open(filepath.Join(g.dir, string(fileInput)))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			out = append(out, g.encode(t, route, wire, bytes.Clone(sc.Bytes())))
		}
	}
	return out
}

func (goldenParityCase) encode(t *testing.T, route []byte, wire *enginetest.WireAdapter, line []byte) []byte {
	if wire == nil {
		return line
	}
	doc, _ := config.Parse(route)
	cfg, _ := doc.Config()
	b, err := wire.Input(t, cfg).Encode(context.Background(), line)
	if err != nil {
		return line
	}
	return b
}

// keys collects top-level member names of the inputs so mutations hit route fields.
func (goldenParityCase) keys(lines [][]byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lines {
		tree, _ := codec.TreeReader{}.Read(l)
		obj, _ := tree.(map[string]any)
		for k := range obj {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	slices.Sort(out)
	return out
}

func TestFastCodecGoldenParity(t *testing.T) {
	fast := 0
	for _, root := range goldenRoots {
		dirs, err := filepath.Glob(filepath.Join(root, "*"))
		if err != nil || len(dirs) == 0 {
			t.Fatalf("no golden cases under %s: %v", root, err)
		}
		for _, dir := range dirs {
			t.Run(filepath.Base(filepath.Dir(dir))+"/"+filepath.Base(dir), func(t *testing.T) {
				if (goldenParityCase{dir: dir}).run(t) {
					fast++
				}
			})
		}
	}
	if fast < int(minFastGoldens) {
		t.Fatalf("only %d golden routes run the fast codec, want >= %d", fast, minFastGoldens)
	}
}
