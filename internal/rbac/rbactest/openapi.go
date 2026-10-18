package rbactest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// OpenAPIContract validates response bodies against the schemas of api/openapi (OpenAPI 3.1 = JSON Schema 2020-12).
type OpenAPIContract struct {
	compiler *jsonschema.Compiler
	root     string
}

// SpecDir is the OpenAPI source folder relative to the repository root.
const SpecDir = "api/openapi"

// MediaType names the response media types of the contract.
type MediaType string

const (
	MediaJSON    MediaType = "application/json"
	MediaProblem MediaType = "application/problem+json"
)

// NewOpenAPIContract loads every YAML file of the spec as a schema resource (cross-file $refs resolve by path).
func NewOpenAPIContract(t *testing.T) *OpenAPIContract {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", SpecDir)
	c := &OpenAPIContract{compiler: jsonschema.NewCompiler(), root: root}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		return c.addFile(path)
	})
	if err != nil {
		t.Fatalf("contract: load spec: %v", err)
	}
	return c
}

func (c *OpenAPIContract) addFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return c.compiler.AddResource("file://"+filepath.ToSlash(path), YAMLNormaliser{}.Normalise(doc))
}

// Check validates one recorded response of the operation `<file>#/<pathKey>/<method>` against its schema.
func (c *OpenAPIContract) Check(t *testing.T, file, pathKey, method string, status int, media MediaType, body []byte) {
	t.Helper()
	ptr := strings.Join([]string{pathKey, strings.ToLower(method), "responses", fmt.Sprint(status), "content",
		strings.ReplaceAll(string(media), "/", "~1"), "schema"}, "/")
	url := "file://" + filepath.ToSlash(filepath.Join(c.root, "paths", file)) + "#/" + ptr
	if ref, ok := c.responseRef(file, pathKey, method, status); ok {
		url = ref + "/content/" + strings.ReplaceAll(string(media), "/", "~1") + "/schema"
	}
	schema, err := c.compiler.Compile(url)
	if err != nil {
		t.Fatalf("contract: compile %s: %v", url, err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("contract: %s %s %d: body is not JSON: %v", method, pathKey, status, err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("contract: %s %s %d violates the spec: %v\nbody: %s", method, pathKey, status, err, body)
	}
}

// responseRef follows a response-level $ref (shared problem responses live in components/problem.yaml).
func (c *OpenAPIContract) responseRef(file, pathKey, method string, status int) (string, bool) {
	path := filepath.Join(c.root, "paths", file)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var doc map[string]any
	if yaml.Unmarshal(raw, &doc) != nil {
		return "", false
	}
	node := YAMLNormaliser{}.Normalise(doc)
	for _, key := range []string{pathKey, strings.ToLower(method), "responses", fmt.Sprint(status)} {
		m, ok := node.(map[string]any)
		if !ok {
			return "", false
		}
		node = m[key]
	}
	m, ok := node.(map[string]any)
	ref, has := m["$ref"].(string)
	if !ok || !has {
		return "", false
	}
	target, frag, _ := strings.Cut(ref, "#")
	return "file://" + filepath.ToSlash(filepath.Join(filepath.Dir(path), target)) + "#" + frag, true
}

// YAMLNormaliser turns yaml.v3 values into JSON-compatible ones (string keys, float numbers).
type YAMLNormaliser struct{}

// Normalise converts maps with non-string keys and integer scalars recursively.
func (n YAMLNormaliser) Normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = n.Normalise(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = n.Normalise(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = n.Normalise(val)
		}
		return out
	case int:
		return float64(t)
	}
	return v
}
