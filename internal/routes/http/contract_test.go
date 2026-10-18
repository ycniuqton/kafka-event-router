package routeshttp_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// SpecLocation names the OpenAPI source of the routes contract (relative to the repository root).
type SpecLocation string

const (
	SpecDir     SpecLocation = "api/openapi"
	RoutesSpec  SpecLocation = "paths/routes.yaml"
	APIPrefix   SpecLocation = "/api/v1"
	ContentJSON SpecLocation = "application/json"
)

// routeOperations maps request paths (after /api/v1) to their path-item key in paths/routes.yaml; first match wins.
var routeOperations = []struct {
	pattern *regexp.Regexp
	key     string
}{
	{regexp.MustCompile(`^/routes$`), "routes"},
	{regexp.MustCompile(`^/routes/bulk$`), "routesBulk"},
	{regexp.MustCompile(`^/routes/export$`), "routesExport"},
	{regexp.MustCompile(`^/routes/import$`), "routesImport"},
	{regexp.MustCompile(`^/routes/[^/]+/unarchive$`), "routeUnarchive"},
	{regexp.MustCompile(`^/routes/[^/]+/export$`), "routeExport"},
	{regexp.MustCompile(`^/routes/[^/]+/replay$`), "routeReplay"},
	{regexp.MustCompile(`^/routes/[^/]+/replays/[^/]+$`), "routeReplayStatus"},
	{regexp.MustCompile(`^/routes/[^/]+/versions/[^/]+/diff/[^/]+$`), "routeDiff"},
	{regexp.MustCompile(`^/routes/[^/]+/versions/[^/]+$`), "routeVersion"},
	{regexp.MustCompile(`^/routes/[^/]+/versions$`), "routeVersions"},
	{regexp.MustCompile(`^/routes/[^/]+/deployments$`), "routeDeployments"},
	{regexp.MustCompile(`^/routes/[^/]+/duplicate$`), "routeDuplicate"},
	{regexp.MustCompile(`^/routes/[^/]+/validate$`), "routeValidate"},
	{regexp.MustCompile(`^/routes/[^/]+/deploy$`), "routeDeploy"},
	{regexp.MustCompile(`^/routes/[^/]+/pause$`), "routePause"},
	{regexp.MustCompile(`^/routes/[^/]+/resume$`), "routeResume"},
	{regexp.MustCompile(`^/routes/[^/]+/rollback$`), "routeRollback"},
	{regexp.MustCompile(`^/routes/[^/]+$`), "route"},
}

// RoutesContract validates recorded responses against paths/routes.yaml (OpenAPI 3.1 = JSON Schema 2020-12)
// and remembers which operations were exercised.
type RoutesContract struct {
	compiler *jsonschema.Compiler
	root     string
	spec     map[string]any
	seen     map[string]bool
}

func newRoutesContract(t *testing.T) *RoutesContract {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", string(SpecDir))
	c := &RoutesContract{compiler: jsonschema.NewCompiler(), root: root, seen: map[string]bool{}}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		doc, err := c.load(path)
		if err == nil && path == filepath.Join(root, string(RoutesSpec)) {
			c.spec, _ = doc.(map[string]any)
		}
		if err != nil {
			return err
		}
		return c.compiler.AddResource("file://"+filepath.ToSlash(path), doc)
	})
	if err != nil || c.spec == nil {
		t.Fatalf("contract: load spec: %v", err)
	}
	return c
}

func (c *RoutesContract) load(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return jsonNormaliser{}.normalise(doc), nil
}

// Check validates one response: the status must be documented and a JSON body must match its schema.
func (c *RoutesContract) Check(t *testing.T, method, path string, status int, media string, body []byte) {
	t.Helper()
	key := c.key(strings.SplitN(strings.TrimPrefix(path, string(APIPrefix)), "?", 2)[0])
	op := strings.ToLower(method)
	c.seen[key+" "+op] = true
	response, url := c.response(key, op, status)
	if response == nil {
		t.Fatalf("contract: %s %s → %d is not documented in %s#/%s/%s/responses", method, path, status, RoutesSpec, key, op)
	}
	if len(body) == 0 {
		return
	}
	mediaKey := strings.TrimSpace(strings.SplitN(media, ";", 2)[0])
	content, _ := response["content"].(map[string]any)
	if _, ok := content[mediaKey]; !ok {
		t.Fatalf("contract: %s %s → %d returns %q, spec lists %v", method, path, status, mediaKey, keys(content))
	}
	if !strings.HasSuffix(mediaKey, "json") {
		return
	}
	schema, err := c.compiler.Compile(url + "/content/" + strings.ReplaceAll(mediaKey, "/", "~1") + "/schema")
	if err != nil {
		t.Fatalf("contract: compile %s: %v", url, err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("contract: %s %s: body is not JSON: %v", method, path, err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("contract: %s %s %d violates the spec: %v\nbody: %s", method, path, status, err, body)
	}
}

func (c *RoutesContract) key(path string) string {
	for _, op := range routeOperations {
		if op.pattern.MatchString(path) {
			return op.key
		}
	}
	return path
}

// response returns the response object (following one $ref into components) and its schema base URL.
func (c *RoutesContract) response(key, op string, status int) (map[string]any, string) {
	file := filepath.Join(c.root, string(RoutesSpec))
	url := fmt.Sprintf("file://%s#/%s/%s/responses/%d", filepath.ToSlash(file), key, op, status)
	node := dig(c.spec, key, op, "responses", fmt.Sprint(status))
	m, _ := node.(map[string]any)
	ref, isRef := m["$ref"].(string)
	if !isRef {
		return m, url
	}
	target, frag, _ := strings.Cut(ref, "#")
	refFile := file
	if target != "" {
		refFile = filepath.Join(filepath.Dir(file), target)
	}
	doc, err := c.load(refFile)
	if err != nil {
		return nil, url
	}
	resolved, _ := dig(doc, strings.Split(strings.TrimPrefix(frag, "/"), "/")...).(map[string]any)
	return resolved, "file://" + filepath.ToSlash(refFile) + "#" + frag
}

// Uncovered lists the documented operations the scenario never called.
func (c *RoutesContract) Uncovered() []string {
	methods := map[string]bool{}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		methods[strings.ToLower(m)] = true
	}
	var out []string
	for key, item := range c.spec {
		ops, _ := item.(map[string]any)
		for op := range ops {
			if methods[op] && !c.seen[key+" "+op] {
				out = append(out, key+" "+op)
			}
		}
	}
	sort.Strings(out)
	return out
}

func dig(node any, path ...string) any {
	for _, k := range path {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = m[k]
	}
	return node
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// jsonNormaliser turns yaml.v3 values into JSON-compatible ones (string keys, float numbers).
type jsonNormaliser struct{}

func (n jsonNormaliser) normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = n.normalise(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = n.normalise(val)
		}
		return out
	case int:
		return float64(t)
	}
	return v
}

// TestRoutesMatchOpenAPI (routes#m-test-contract) drives every MVP routes endpoint through the real handlers and
// validates each status and body against api/openapi/paths/routes.yaml; every documented operation must be called.
func TestRoutesMatchOpenAPI(t *testing.T) {
	h, contract := newHarness(t), newRoutesContract(t)
	h.observe = func(method, path string, status int, media string, body []byte) {
		contract.Check(t, method, path, status, media, body)
	}
	id := h.runLifecycle()
	h.expect(h.do(http.MethodGet, base+"/"+id, nil, ""), http.StatusOK)
	h.expect(h.do(http.MethodGet, base+"/"+id+"/versions/99", nil, ""), http.StatusNotFound)
	if missing := contract.Uncovered(); len(missing) > 0 {
		t.Fatalf("operations never exercised: %v", missing)
	}
}
