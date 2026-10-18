package contract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// Violation is one request/response that does not match the spec.
type Violation struct {
	Method string
	Path   string
	Status int
	Reason string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s %s → %d: %s", v.Method, v.Path, v.Status, v.Reason)
}

// Reason names the non-schema violation kinds.
type Reason string

const ReasonUndocumented Reason = "operation not documented in the OpenAPI spec"

// Validator checks exchanges against the spec: request params/body and response status/headers/body.
type Validator struct {
	spec    *Spec
	options *openapi3filter.Options
}

// NewValidator builds a validator; authentication is not re-checked (the handler chain does that).
func NewValidator(spec *Spec) *Validator {
	return &Validator{spec: spec, options: &openapi3filter.Options{
		AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, MultiError: true,
	}}
}

// Check validates one exchange and returns its violations (none when it conforms).
func (v *Validator) Check(ctx context.Context, req *http.Request, reqBody []byte, status int, header http.Header, respBody []byte) []Violation {
	route, params, ok := v.spec.Route(req.Method, req.URL.Path)
	if !ok {
		return []Violation{{Method: req.Method, Path: req.URL.Path, Status: status, Reason: string(ReasonUndocumented)}}
	}
	clone := req.Clone(ctx)
	clone.Body = io.NopCloser(bytes.NewReader(reqBody))
	in := &openapi3filter.RequestValidationInput{Request: clone, PathParams: params, Route: route, Options: v.options}
	var out []Violation
	if err := openapi3filter.ValidateRequest(ctx, in); err != nil && status < http.StatusBadRequest {
		out = append(out, Violation{Method: req.Method, Path: req.URL.Path, Status: status, Reason: "request: " + err.Error()})
	}
	resp := &openapi3filter.ResponseValidationInput{RequestValidationInput: in, Status: status, Header: header,
		Body: io.NopCloser(bytes.NewReader(respBody)), Options: v.options}
	if err := openapi3filter.ValidateResponse(ctx, resp); err != nil {
		out = append(out, Violation{Method: req.Method, Path: req.URL.Path, Status: status, Reason: "response: " + err.Error()})
	}
	return out
}

// Recorder is an http.Handler wrapper for API tests: every exchange is validated and violations collected.
type Recorder struct {
	next      http.Handler
	validator *Validator
	mu        sync.Mutex
	found     []Violation
	exchanges int
}

// NewRecorder wraps next.
func NewRecorder(next http.Handler, validator *Validator) *Recorder {
	return &Recorder{next: next, validator: validator}
}

// ServeHTTP serves through next, then validates the captured exchange (redirects are not validated).
func (r *Recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.next.ServeHTTP(rec, req)
	for k, vs := range rec.Header() {
		w.Header()[k] = vs
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
	violations := r.validator.Check(context.Background(), req, body, rec.Code, rec.Header(), rec.Body.Bytes())
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exchanges++
	r.found = append(r.found, violations...)
}

// Violations returns every violation seen so far.
func (r *Recorder) Violations() []Violation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Violation(nil), r.found...)
}

// Exchanges returns how many exchanges were validated.
func (r *Recorder) Exchanges() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exchanges
}
