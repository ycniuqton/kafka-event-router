package portability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/portability"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
)

var update = flag.Bool("update", false, "rewrite the golden export files")

const (
	srcID  = "11111111-1111-1111-1111-111111111111"
	dstID  = "22222222-2222-2222-2222-222222222222"
	teamID = "team-a"
)

// directory is an in-memory ConnectionDirectory with two connections of team-a.
type directory map[string]validation.ConnectionInfo

func (d directory) Find(_ context.Context, ids []string) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	for _, id := range ids {
		if c, ok := d[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (d directory) FindByNames(_ context.Context, _ string, names []string) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	for _, c := range d {
		for _, n := range names {
			if c.Name == n {
				out[n] = c
			}
		}
	}
	return out, nil
}

var conns = directory{srcID: {ID: srcID, TeamID: teamID, Name: "orders-prod"}, dstID: {ID: dstID, TeamID: teamID, Name: "partner"}}

// TestGoldenYAMLRoundTrip (routes#p2-test-portability): export (ids → names, YAML) matches the golden file and
// importing it back (YAML → JSON, names → ids) gives the original document.
func TestGoldenYAMLRoundTrip(t *testing.T) {
	codecs, err := portability.DefaultCodecs()
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := filepath.Glob("testdata/*.json")
	if len(inputs) == 0 {
		t.Fatal("no golden inputs")
	}
	for _, in := range inputs {
		t.Run(filepath.Base(in), func(t *testing.T) {
			original, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			exported := exportYAML(t, codecs, original)
			golden := strings.TrimSuffix(in, ".json") + ".yaml"
			if *update {
				_ = os.WriteFile(golden, exported, 0o644)
			}
			want, _ := os.ReadFile(golden)
			if !bytes.Equal(exported, want) {
				t.Fatalf("export differs from %s:\n%s", golden, exported)
			}
			assertSameDocument(t, original, importYAML(t, codecs, exported))
		})
	}
}

func exportYAML(t *testing.T, codecs *portability.CodecManager, raw []byte) []byte {
	t.Helper()
	tree, err := portability.ParseTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := (portability.ConnectionResolver{Directory: conns}).ToNames(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(t, tree)), srcID) {
		t.Fatal("connection id left in the export")
	}
	codec, _ := codecs.Codec(routes.FormatYAML)
	out, err := codec.Encode([]json.RawMessage{mustJSON(t, tree)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func importYAML(t *testing.T, codecs *portability.CodecManager, payload []byte) []byte {
	t.Helper()
	codec, _ := codecs.Codec(routes.FormatYAML)
	docs, err := codec.Decode(payload)
	if err != nil || len(docs) != 1 {
		t.Fatalf("decode: %v (%d docs)", err, len(docs))
	}
	tree, err := portability.ParseTree(docs[0])
	if err != nil {
		t.Fatal(err)
	}
	issues, err := (portability.ConnectionResolver{Directory: conns}).ToIDs(context.Background(), teamID, tree)
	if err != nil || len(issues) > 0 {
		t.Fatalf("resolve: %v %v", issues, err)
	}
	return mustJSON(t, tree)
}

func mustJSON(t *testing.T, tree portability.DocumentTree) []byte {
	t.Helper()
	raw, err := tree.JSON()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertSameDocument(t *testing.T, a, b []byte) {
	t.Helper()
	ca, errA := config.Canonicalizer{}.Canonical(a)
	cb, errB := config.Canonicalizer{}.Canonical(b)
	if errA != nil || errB != nil || !bytes.Equal(ca, cb) {
		t.Fatalf("round trip changed the document:\n%s\n%s", ca, cb)
	}
}

// TestMultiDocumentStreams: YAML joins with separators, skips empty documents; JSON writes an array.
func TestMultiDocumentStreams(t *testing.T) {
	codecs, _ := portability.DefaultCodecs()
	docs := []json.RawMessage{json.RawMessage(`{"name":"a"}`), json.RawMessage(`{"name":"b"}`)}
	y, _ := codecs.Codec(routes.FormatYAML)
	enc, err := y.Encode(docs)
	if err != nil || string(enc) != "---\nname: a\n---\nname: b\n" {
		t.Fatalf("%q %v", enc, err)
	}
	back, err := y.Decode(append([]byte("---\n---\n"), enc...))
	if err != nil || len(back) != 2 || string(back[1]) != `{"name":"b"}` {
		t.Fatalf("%s %v", back, err)
	}
	j, _ := codecs.Codec(routes.FormatJSON)
	arr, _ := j.Encode(docs)
	if parsed, err := j.Decode(arr); err != nil || len(parsed) != 2 {
		t.Fatalf("%s %v", arr, err)
	}
	if _, err := codecs.Codec("xml"); err != portability.ErrUnknownFormat {
		t.Fatal(err)
	}
}

// TestUnknownConnectionNameIsAnIssue: unresolvable references are reported at their JSON Pointer.
func TestUnknownConnectionNameIsAnIssue(t *testing.T) {
	tree, _ := portability.ParseTree([]byte(`{"name":"x","source":{"connectionId":"ghost"},"branches":[{"id":"main","destination":{"connectionId":"partner","topic":"t"}}]}`))
	issues, err := (portability.ConnectionResolver{Directory: conns}).ToIDs(context.Background(), teamID, tree)
	if err != nil || len(issues) != 1 || issues[0].Path != "/source/connectionId" || issues[0].Code != config.IssueConnectionNotFound {
		t.Fatalf("%+v %v", issues, err)
	}
	if !strings.Contains(string(mustJSON(t, tree)), dstID) {
		t.Fatal("known name not resolved")
	}
}

// FuzzDecodeNeverPanics (routes#p2-test-portability): malformed YAML / JSON payloads fail with an error, never a panic.
func FuzzDecodeNeverPanics(f *testing.F) {
	for _, seed := range []string{"a: [", "---\n- 1\n- {", "*anchor", "name: &a [*a]", "{\"name\":", "!!binary x", "? [a]\n: b",
		"a: &x {b: *x}", strings.Repeat("[", 200), "\t- x", "name: x\nsource: {connectionId: [1]}", "--- !!str\n"} {
		f.Add([]byte(seed))
	}
	codecs, _ := portability.DefaultCodecs()
	f.Fuzz(func(t *testing.T, payload []byte) {
		for _, format := range []routes.DocumentFormat{routes.FormatYAML, routes.FormatJSON} {
			codec, _ := codecs.Codec(format)
			docs, err := codec.Decode(payload)
			if err != nil {
				continue
			}
			for _, d := range docs {
				if tree, err := portability.ParseTree(d); err == nil {
					_ = tree.Slots()
					_, _ = tree.JSON()
				}
			}
		}
	})
}
