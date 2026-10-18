package diff

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

// goldenCase is one testdata file: two configs (null = empty side) and the expected diff.
type goldenCase struct {
	A        json.RawMessage `json:"a"`
	B        json.RawMessage `json:"b"`
	Expected *RouteDiff      `json:"expected"`
}

func decode(t *testing.T, raw json.RawMessage) *config.RouteConfig {
	t.Helper()
	if raw == nil || string(raw) == "null" {
		return nil
	}
	var c config.RouteConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestGoldenDiffs(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.json")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := json.Unmarshal(raw, &gc); err != nil {
				t.Fatal(err)
			}
			got := DefaultRouteDiffer().Diff("a", "b", decode(t, gc.A), decode(t, gc.B))
			if *update {
				gc.Expected = &got
				out, _ := json.MarshalIndent(gc, "", "  ")
				_ = os.WriteFile(f, append(out, '\n'), 0o644)
				return
			}
			want, _ := json.Marshal(gc.Expected)
			have, _ := json.Marshal(got)
			if string(want) != string(have) {
				t.Fatalf("diff mismatch\nwant %s\nhave %s", want, have)
			}
		})
	}
}

func TestEntryComparer(t *testing.T) {
	a := []Entry{{Key: "x", Text: "x 1"}, {Key: "y", Text: "y"}, {Key: "z", Text: "z"}}
	b := []Entry{{Key: "x", Text: "x 2"}, {Key: "z", Text: "z"}, {Key: "w", Text: "w"}}
	lines := EntryComparer{}.Compare(a, b)
	var got []string
	for _, l := range lines {
		got = append(got, string(l.Mark)+l.Text)
	}
	if strings.Join(got, "|") != "~x 1 → x 2|-y|+w" {
		t.Fatal(got)
	}
}

func TestEmptySideIsAllAdded(t *testing.T) {
	cfg := &config.RouteConfig{Name: "r", Source: config.Source{ConnectionID: "c", Topics: []string{"t"}, Format: config.Format{Type: config.FormatTypeJSON}},
		Branches: []config.Branch{{ID: "main", Destination: config.Destination{ConnectionID: "d", Topic: "o"}}}}
	d := DefaultRouteDiffer().Diff("active", "draft", nil, cfg)
	if !d.Changed() {
		t.Fatal("expected changes")
	}
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			if l.Mark != MarkAdded {
				t.Fatalf("%+v", l)
			}
		}
	}
	if DefaultRouteDiffer().Diff("a", "b", cfg, cfg).Changed() {
		t.Fatal("identical configs have no changes")
	}
}
