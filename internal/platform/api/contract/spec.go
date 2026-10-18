// Package contract validates real requests and responses of API handlers against the OpenAPI spec
// (api/openapi/openapi.yaml, kin-openapi with JSON Schema 2020-12) and reports routes missing from it.
package contract

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
)

// SpecLocation names where the spec lives relative to the module root.
type SpecLocation string

const (
	DefaultSpecPath SpecLocation = "api/openapi/openapi.yaml"
	ModuleMarker    SpecLocation = "go.mod"
)

// Spec is the loaded OpenAPI document plus an index from concrete request paths to operations.
type Spec struct {
	doc    *openapi3.T
	routes []indexedPath
}

type indexedPath struct {
	full     string
	pattern  *regexp.Regexp
	template string
	item     *openapi3.PathItem
	server   *openapi3.Server
}

// PathPattern names the placeholder used while turning path templates into regular expressions.
type PathPattern string

const ParamPlaceholder PathPattern = "\x00"

// paramSegment matches `{name}` path parameters (chi patterns may carry `{name:regex}`).
var paramSegment = regexp.MustCompile(`\{([^}:]+)(:[^}]*)?\}`)

// LoadDefault finds the module root from the working directory and loads the spec there.
func LoadDefault(ctx context.Context) (*Spec, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, string(ModuleMarker))); err == nil {
			return Load(ctx, filepath.Join(dir, string(DefaultSpecPath)))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, errors.New("contract: go.mod not found above the working directory")
		}
		dir = parent
	}
}

// Load reads the spec with its external $refs (paths/*.yaml, components/*.yaml).
func Load(ctx context.Context, path string) (*Spec, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.Context = ctx
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("contract: load %s: %w", path, err)
	}
	s := &Spec{doc: doc}
	for template, item := range doc.Paths.Map() {
		s.add(template, item)
	}
	return s, nil
}

func (s *Spec) add(template string, item *openapi3.PathItem) {
	server := s.serverOf(item)
	prefix := ""
	if u, err := url.Parse(server.URL); err == nil {
		prefix = strings.TrimRight(u.Path, "/")
	}
	full := prefix + template
	quoted := regexp.QuoteMeta(paramSegment.ReplaceAllString(full, string(ParamPlaceholder)))
	re := regexp.MustCompile("^" + strings.ReplaceAll(quoted, string(ParamPlaceholder), `[^/]+`) + "$")
	s.routes = append(s.routes, indexedPath{full: full, pattern: re, template: template, item: item, server: server})
}

func (s *Spec) serverOf(item *openapi3.PathItem) *openapi3.Server {
	if len(item.Servers) > 0 {
		return item.Servers[0]
	}
	return s.doc.Servers[0]
}

// Doc returns the loaded document.
func (s *Spec) Doc() *openapi3.T {
	return s.doc
}

// Route resolves a method and concrete request path (e.g. /api/v1/auth/tokens/123) to its operation.
func (s *Spec) Route(method, path string) (*routers.Route, map[string]string, bool) {
	for _, p := range s.routes {
		if !p.pattern.MatchString(path) {
			continue
		}
		op := p.item.GetOperation(strings.ToUpper(method))
		if op == nil {
			return nil, nil, false
		}
		route := &routers.Route{Spec: s.doc, Server: p.server, Path: p.template, PathItem: p.item, Method: strings.ToUpper(method), Operation: op}
		return route, s.params(p.full, path), true
	}
	return nil, nil, false
}

// Documents reports whether a mounted chi route (method + pattern) has an operation in the spec.
func (s *Spec) Documents(method, chiPattern string) bool {
	want := paramSegment.ReplaceAllString(chiPattern, "{}")
	for _, p := range s.routes {
		if paramSegment.ReplaceAllString(p.full, "{}") == want {
			return p.item.GetOperation(strings.ToUpper(method)) != nil
		}
	}
	return false
}

func (s *Spec) params(template, path string) map[string]string {
	out := map[string]string{}
	tSegs, pSegs := strings.Split(template, "/"), strings.Split(path, "/")
	for i, seg := range tSegs {
		if m := paramSegment.FindStringSubmatch(seg); m != nil && i < len(pSegs) {
			out[m[1]] = pSegs[i]
		}
	}
	return out
}
