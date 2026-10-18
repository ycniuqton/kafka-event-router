// Package problem renders RFC 7807 problem responses. Details carries `path` (a JSON Pointer),
// `errors[]`, `request_id` and `current_etag` so the UI can highlight fields and resolve conflicts.
package problem

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// MediaType is the content type of RFC 7807 problem documents.
type MediaType string

const MediaTypeProblemJSON MediaType = "application/problem+json"

// Header names HTTP headers written with problem responses.
type Header string

const (
	HeaderContentType Header = "Content-Type"
	HeaderRetryAfter  Header = "Retry-After"
)

// TypeURI identifies a problem category; "about:blank" means the HTTP status says it all.
type TypeURI string

const (
	TypeBlank                TypeURI = "about:blank"
	TypeValidation           TypeURI = "https://eventrouter.dev/problems/validation"
	TypeMalformedBody        TypeURI = "https://eventrouter.dev/problems/malformed-body"
	TypeUnauthenticated      TypeURI = "https://eventrouter.dev/problems/unauthenticated"
	TypeForbidden            TypeURI = "https://eventrouter.dev/problems/forbidden"
	TypeNotFound             TypeURI = "https://eventrouter.dev/problems/not-found"
	TypeMethodNotAllowed     TypeURI = "https://eventrouter.dev/problems/method-not-allowed"
	TypeConflict             TypeURI = "https://eventrouter.dev/problems/conflict"
	TypePreconditionFailed   TypeURI = "https://eventrouter.dev/problems/precondition-failed"
	TypePayloadTooLarge      TypeURI = "https://eventrouter.dev/problems/payload-too-large"
	TypePreconditionRequired TypeURI = "https://eventrouter.dev/problems/precondition-required"
	TypeRateLimited          TypeURI = "https://eventrouter.dev/problems/rate-limited"
	TypeUnavailable          TypeURI = "https://eventrouter.dev/problems/unavailable"
	TypeInternal             TypeURI = "https://eventrouter.dev/problems/internal"
)

// FieldError points at one offending request field by JSON Pointer.
type FieldError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Details is an RFC 7807 problem document.
type Details struct {
	Type              TypeURI      `json:"type"`
	Title             string       `json:"title"`
	Status            int          `json:"status"`
	Detail            string       `json:"detail,omitempty"`
	Instance          string       `json:"instance,omitempty"`
	Path              string       `json:"path,omitempty"`
	RequestID         string       `json:"request_id,omitempty"`
	CurrentETag       string       `json:"current_etag,omitempty"`
	Errors            []FieldError `json:"errors,omitempty"`
	Code              string       `json:"code,omitempty"`
	MissingPermission string       `json:"missingPermission,omitempty"`
	RetryAfter        int          `json:"retry_after,omitempty"`
}

// New builds a problem for the given status with the standard status text as title.
func New(typ TypeURI, status int, detail string) Details {
	return Details{Type: typ, Title: http.StatusText(status), Status: status, Detail: detail}
}

// WithPath returns a copy pointing at the offending JSON Pointer of the request.
func (d Details) WithPath(path string) Details {
	d.Path = path
	return d
}

// WithErrors returns a copy carrying field errors; path defaults to the first error's path.
func (d Details) WithErrors(errs []FieldError) Details {
	d.Errors = errs
	if d.Path == "" && len(errs) > 0 {
		d.Path = errs[0].Path
	}
	return d
}

// RequestIDSource resolves the current request id (implemented by the api middleware).
type RequestIDSource func(r *http.Request) string

// Writer serialises problem documents onto HTTP responses.
type Writer struct {
	RequestID RequestIDSource
}

// Write sends the problem with the correct media type and status code.
func (wr Writer) Write(w http.ResponseWriter, r *http.Request, d Details) {
	if d.Instance == "" && r != nil {
		d.Instance = r.URL.Path
	}
	if d.RequestID == "" && r != nil && wr.RequestID != nil {
		d.RequestID = wr.RequestID(r)
	}
	w.Header().Set(string(HeaderContentType), string(MediaTypeProblemJSON))
	if d.RetryAfter > 0 {
		w.Header().Set(string(HeaderRetryAfter), strconv.Itoa(d.RetryAfter))
	}
	w.WriteHeader(d.Status)
	_ = json.NewEncoder(w).Encode(d)
}

// NotFoundHandler answers unknown routes with a problem document.
type NotFoundHandler struct{ Writer }

func (h NotFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Write(w, r, New(TypeNotFound, http.StatusNotFound, "no such resource"))
}

// MethodNotAllowedHandler answers unsupported methods with a problem document.
type MethodNotAllowedHandler struct{ Writer }

func (h MethodNotAllowedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Write(w, r, New(TypeMethodNotAllowed, http.StatusMethodNotAllowed, "method not supported on this resource"))
}
