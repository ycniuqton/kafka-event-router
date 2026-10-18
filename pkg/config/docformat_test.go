package config_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

func compact(t *testing.T, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestYAMLRoundTripGoldenRoutes converts every golden route JSON → YAML → JSON and
// requires identical bytes (member order, number text) and an identical config hash.
func TestYAMLRoundTripGoldenRoutes(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "engine", "testdata", "golden", "*", "route.json"))
	if len(files) < 10 {
		t.Fatalf("golden routes: %d", len(files))
	}
	m := config.DefaultDocumentFormats()
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		y, err := m.Convert(config.DocFormatJSON, config.DocFormatYAML, raw)
		if err != nil {
			t.Fatalf("%s: to yaml: %v", f, err)
		}
		back, err := m.Convert(config.DocFormatYAML, config.DocFormatJSON, y)
		if err != nil {
			t.Fatalf("%s: from yaml: %v\n%s", f, err, y)
		}
		if !bytes.Equal(compact(t, back), compact(t, raw)) {
			t.Fatalf("%s: round trip differs\n%s\n%s", f, compact(t, raw), compact(t, back))
		}
		h1, _ := config.Canonicalizer{}.Hash(raw)
		h2, _ := config.Canonicalizer{}.Hash(back)
		if h1 != h2 {
			t.Fatalf("%s: hash differs", f)
		}
	}
}

func TestYAMLEdgeValues(t *testing.T) {
	raw := []byte(`{"z":"true","a":"123","n":null,"b":false,"big":12345678901234567890,"f":1.50,"e":1e3,"s":"multi\nline: x","empty":"","list":[],"obj":{},"k: colon":"#hash","unicode":"äö"}`)
	f := config.YAMLDocFormat{}
	y, err := f.FromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(y), "z:") > strings.Index(string(y), "a:") {
		t.Fatalf("key order lost:\n%s", y)
	}
	back, err := f.ToJSON(y)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, raw) {
		t.Fatalf("round trip\n%s\n%s\n%s", raw, back, y)
	}
}

func TestYAMLAuthoredInput(t *testing.T) {
	y := []byte("apiVersion: router/v1\nname: from-yaml\nlimits: &l\n  maxInFlight: 0x10\nsource:\n  connectionId: c1\n  format: {type: json}\ncopy: *l\nyes_value: yes\n")
	got, err := config.YAMLDocFormat{}.ToJSON(y)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"apiVersion":"router/v1","name":"from-yaml","limits":{"maxInFlight":16},"source":{"connectionId":"c1","format":{"type":"json"}},"copy":{"maxInFlight":16},"yes_value":"yes"}`
	if string(got) != want {
		t.Fatalf("got %s", got)
	}
	for _, bad := range []string{"a: 1\n---\nb: 2\n", "a: [1\n", "x: .inf\n", "<<: {a: 1}\n", ""} {
		if _, err := (config.YAMLDocFormat{}).ToJSON([]byte(bad)); !errors.Is(err, config.ErrDocFormat) {
			t.Errorf("%q: want error, got %v", bad, err)
		}
	}
}

func TestDocumentFormatManager(t *testing.T) {
	m := config.DefaultDocumentFormats()
	if _, err := m.Get("toml"); !errors.Is(err, config.ErrDocFormat) {
		t.Fatal(err)
	}
	j, _ := m.Get(config.DocFormatJSON)
	out, err := j.FromJSON([]byte(`{"b":1,"a":[1,2]}`))
	if err != nil || string(out) != "{\n  \"b\": 1,\n  \"a\": [\n    1,\n    2\n  ]\n}\n" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := j.ToJSON([]byte(`{"a":1} x`)); !errors.Is(err, config.ErrDocFormat) {
		t.Fatal("trailing data accepted")
	}
	y, _ := m.Get(config.DocFormatYAML)
	if y.MediaType() != config.MediaTypeYAML || j.MediaType() != config.MediaTypeJSON {
		t.Fatal("media types")
	}
}
