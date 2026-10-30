package enrich

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// HTTPRequest compiles `http` requests: method, path (escaped placeholders), query, headers, JSON body.
type HTTPRequest struct{}

func (HTTPRequest) Kind() config.LookupRequestKind { return config.LookupRequestKindHttp }

func (HTTPRequest) Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	b := &httpBuilder{method: string(config.LookupHttpMethodGET)}
	if raw.Method != nil {
		b.method = string(*raw.Method)
	}
	var okP bool
	b.path, okP = c.Template(PointerText{}.Of(raw.Path), config.TokenPath)
	q, okQ := c.Map(raw.Query, TokenQuery)
	h, okH := c.Map(raw.Headers, TokenHeaders)
	b.query, b.headers = q, h
	okB := true
	if raw.Body != nil {
		body, err := BodyTemplateParser{}.Parse(step.NumberNormalizer{}.Of(raw.Body))
		if err != nil {
			c.Fail(config.IssuePathInvalid, TokenBody, "%v", err)
		}
		b.body, b.hasBody, okB = body, true, err == nil
	}
	return b, okP && okQ && okH && okB
}

type httpBuilder struct {
	method         string
	path           Template
	query, headers TemplateMap
	body           BodyTemplate
	hasBody        bool
}

func (b *httpBuilder) Kind() ctxhelpers.SourceRequestKind { return ctxhelpers.RequestHTTP }

func (b *httpBuilder) Render(ev *engine.Event) (ctxhelpers.SourceRequest, bool) {
	p, okP := b.path.Text(ev, PathSegment{})
	q, okQ := b.query.Text(ev)
	h, okH := b.headers.Text(ev)
	req := ctxhelpers.SourceRequest{Method: b.method, Path: p, Query: q, Headers: h}
	okB := true
	if b.hasBody {
		req.Body, okB = b.body.Render(ev)
	}
	return req, okP && okQ && okH && okB
}

func (b *httpBuilder) Fields() []path.FieldPath {
	out := append(b.path.Fields(), b.query.Fields()...)
	out = append(out, b.headers.Fields()...)
	return append(out, b.body.Fields()...)
}

// RedisRequest compiles `redis` requests: get key, hget key + hashField, mget keys.
type RedisRequest struct{}

func (RedisRequest) Kind() config.LookupRequestKind { return config.LookupRequestKindRedis }

func (RedisRequest) Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	if raw.Op == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenOp, "op is required for redis requests")
		return nil, false
	}
	shape, ok := RedisShapes[*raw.Op]
	if !ok {
		c.Fail(config.IssueSchemaInvalid, config.TokenOp, "redis op %q is not supported", *raw.Op)
		return nil, false
	}
	return shape.compile(c, raw)
}

// redisShape is the member layout of one redis op.
type redisShape struct {
	op        config.LookupRedisOp
	key, hash bool
	keys      bool
}

// RedisShapes lists the members each redis op uses.
var RedisShapes = map[config.LookupRedisOp]redisShape{
	config.LookupRedisOpGet:  {op: config.LookupRedisOpGet, key: true},
	config.LookupRedisOpHget: {op: config.LookupRedisOpHget, key: true, hash: true},
	config.LookupRedisOpMget: {op: config.LookupRedisOpMget, keys: true},
}

func (s redisShape) compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	b := &redisBuilder{op: string(s.op)}
	ok := true
	if s.key {
		var okK bool
		b.key, okK = c.Required(raw.Key, config.TokenKey)
		ok = ok && okK
	}
	if s.hash {
		var okH bool
		b.hash, okH = c.Required(raw.HashField, TokenHashField)
		ok = ok && okH
	}
	if s.keys {
		ok = ok && b.compileKeys(c, raw.Keys)
	}
	return b, ok
}

type redisBuilder struct {
	op        string
	key, hash Template
	keys      TemplateList
}

func (b *redisBuilder) compileKeys(c RequestCompileCtx, raw []string) bool {
	if len(raw) == 0 {
		c.Fail(config.IssueSchemaInvalid, TokenKeys, "keys are required for mget")
		return false
	}
	keys, idx, err := TemplateListParser{}.Parse(raw)
	if err != nil {
		c.Step.Scope.Fail(config.IssuePathInvalid, c.Pointer.Token(TokenKeys).Index(idx), "%v", err)
		return false
	}
	b.keys = keys
	return true
}

func (b *redisBuilder) Kind() ctxhelpers.SourceRequestKind { return ctxhelpers.RequestRedis }

func (b *redisBuilder) Render(ev *engine.Event) (ctxhelpers.SourceRequest, bool) {
	req := ctxhelpers.SourceRequest{Op: b.op}
	okK, okH, okL := true, true, true
	if len(b.key.parts) > 0 {
		req.Key, okK = b.key.Text(ev, PlainText{})
	}
	if len(b.hash.parts) > 0 {
		req.HashField, okH = b.hash.Text(ev, PlainText{})
	}
	if len(b.keys) > 0 {
		req.Keys, okL = b.keys.Text(ev)
	}
	return req, okK && okH && okL
}

func (b *redisBuilder) Fields() []path.FieldPath {
	out := append(b.key.Fields(), b.hash.Fields()...)
	return append(out, b.keys.Fields()...)
}

// SQLRequest compiles `sql` requests: one SELECT with `:name` params bound from templates.
type SQLRequest struct{}

func (SQLRequest) Kind() config.LookupRequestKind { return config.LookupRequestKindSql }

func (SQLRequest) Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	if raw.Sql == nil {
		c.Fail(config.IssueSchemaInvalid, TokenSQL, "sql is required for sql requests")
		return nil, false
	}
	stmt, err := SQLParams{}.Rewrite(*raw.Sql)
	if err != nil {
		c.Fail(config.IssueSchemaInvalid, TokenSQL, "%v", err)
		return nil, false
	}
	params, ok := c.Map(raw.Params, TokenParams)
	if !ok {
		return nil, false
	}
	b := &sqlBuilder{sql: stmt.Text}
	for _, name := range stmt.Names {
		t, found := params[name]
		if !found {
			c.Step.Scope.Fail(config.IssueSchemaInvalid, c.Pointer.Token(TokenParams).Key(name), "parameter :%s has no value in params", name)
			ok = false
		}
		b.params = append(b.params, t)
	}
	return b, ok
}

type sqlBuilder struct {
	sql    string
	params []Template
}

func (b *sqlBuilder) Kind() ctxhelpers.SourceRequestKind { return ctxhelpers.RequestSQL }

func (b *sqlBuilder) Render(ev *engine.Event) (ctxhelpers.SourceRequest, bool) {
	args := make([]any, 0, len(b.params))
	for _, t := range b.params {
		v, ok := t.Value(ev)
		if !ok {
			return ctxhelpers.SourceRequest{}, false
		}
		args = append(args, v)
	}
	return ctxhelpers.SourceRequest{SQL: b.sql, Params: args}, true
}

func (b *sqlBuilder) Fields() []path.FieldPath {
	var out []path.FieldPath
	for _, t := range b.params {
		out = append(out, t.Fields()...)
	}
	return out
}

// StaticRequest compiles `static` requests: the key of a library lookup table row.
type StaticRequest struct{}

func (StaticRequest) Kind() config.LookupRequestKind { return config.LookupRequestKindStatic }

func (StaticRequest) Compile(c RequestCompileCtx, raw config.LookupRequest) (RequestBuilder, bool) {
	key, ok := c.Required(raw.Key, config.TokenKey)
	return &staticBuilder{key: key}, ok
}

type staticBuilder struct {
	key Template
}

func (b *staticBuilder) Kind() ctxhelpers.SourceRequestKind { return ctxhelpers.RequestStatic }

func (b *staticBuilder) Render(ev *engine.Event) (ctxhelpers.SourceRequest, bool) {
	k, ok := b.key.Text(ev, PlainText{})
	return ctxhelpers.SourceRequest{Key: k}, ok
}

func (b *staticBuilder) Fields() []path.FieldPath { return b.key.Fields() }

// PointerText dereferences optional document strings.
type PointerText struct{}

// Of returns *s or "".
func (PointerText) Of(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
