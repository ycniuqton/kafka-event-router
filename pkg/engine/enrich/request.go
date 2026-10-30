package enrich

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Document members of the lookup source mode used in issue pointers.
const (
	TokenLookup    config.PointerToken = "lookup"
	TokenSourceID  config.PointerToken = "sourceId"
	TokenRequest   config.PointerToken = "request"
	TokenMap       config.PointerToken = "map"
	TokenQuery     config.PointerToken = "query"
	TokenHeaders   config.PointerToken = "headers"
	TokenBody      config.PointerToken = "body"
	TokenKeys      config.PointerToken = "keys"
	TokenHashField config.PointerToken = "hashField"
	TokenSQL       config.PointerToken = "sql"
	TokenParams    config.PointerToken = "params"
	TokenOnMiss    config.PointerToken = "onMiss"
	TokenOnError   config.PointerToken = "onError"
	TokenAction    config.PointerToken = "action"
)

// RequestBuilder renders the request of one event; ok is false when a placeholder field is missing (a miss).
type RequestBuilder interface {
	Kind() ctxhelpers.SourceRequestKind
	Render(ev *engine.Event) (ctxhelpers.SourceRequest, bool)
	Fields() []path.FieldPath
}

// RequestCompiler compiles one request kind; register implementations in a RequestCompilerManager.
type RequestCompiler interface {
	Kind() config.LookupRequestKind
	Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool)
}

// RequestCompileCtx is the compile state of one request (issues go under its pointer).
type RequestCompileCtx struct {
	Step    *step.StepCompileCtx
	Pointer config.Pointer
}

// Fail records an error issue on a request member.
func (c RequestCompileCtx) Fail(code config.IssueCode, member config.PointerToken, format string, args ...any) {
	c.Step.Scope.Fail(code, c.Pointer.Token(member), format, args...)
}

// Required compiles a required template member.
func (c RequestCompileCtx) Required(raw *string, member config.PointerToken) (Template, bool) {
	if raw == nil || *raw == "" {
		c.Fail(config.IssueSchemaInvalid, member, "%s is required for this request kind", member)
		return Template{}, false
	}
	return c.Template(*raw, member)
}

// Template compiles one template member.
func (c RequestCompileCtx) Template(raw string, member config.PointerToken) (Template, bool) {
	t, err := TemplateParser{}.Parse(raw)
	if err != nil {
		c.Fail(config.IssuePathInvalid, member, "%v", err)
		return Template{}, false
	}
	return t, true
}

// Map compiles a template map member.
func (c RequestCompileCtx) Map(raw map[string]string, member config.PointerToken) (TemplateMap, bool) {
	m, name, err := TemplateMapParser{}.Parse(raw)
	if err != nil {
		c.Step.Scope.Fail(config.IssuePathInvalid, c.Pointer.Token(member).Key(name), "%v", err)
		return nil, false
	}
	return m, true
}

// RequestCompilerManager selects request compilers by kind; the lookup compiler talks only to it.
type RequestCompilerManager struct {
	compilers map[config.LookupRequestKind]RequestCompiler
}

// NewRequestCompilerManager registers compilers.
func NewRequestCompilerManager(cs ...RequestCompiler) *RequestCompilerManager {
	m := &RequestCompilerManager{compilers: map[config.LookupRequestKind]RequestCompiler{}}
	for _, rc := range cs {
		m.compilers[rc.Kind()] = rc
	}
	return m
}

// DefaultRequestCompilers registers http, redis, sql and static requests.
func DefaultRequestCompilers() *RequestCompilerManager {
	return NewRequestCompilerManager(HTTPRequest{}, RedisRequest{}, SQLRequest{}, StaticRequest{})
}

// Compile dispatches by request kind.
func (m *RequestCompilerManager) Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	rc, ok := m.compilers[raw.Kind]
	if !ok {
		c.Fail(config.IssueSchemaInvalid, config.TokenKind, "lookup request kind %q is not supported", raw.Kind)
		return nil, false
	}
	return rc.Compile(c, raw)
}
