package problem

import (
	"errors"
	"net/http"
)

// Kind classifies a domain error; the Catalog maps each kind to a TypeURI and HTTP status.
type Kind string

const (
	KindValidation           Kind = "validation"
	KindMalformedBody        Kind = "malformed_body"
	KindUnauthenticated      Kind = "unauthenticated"
	KindForbidden            Kind = "forbidden"
	KindNotFound             Kind = "not_found"
	KindConflict             Kind = "conflict"
	KindPreconditionFailed   Kind = "precondition_failed"
	KindPayloadTooLarge      Kind = "payload_too_large"
	KindPreconditionRequired Kind = "precondition_required"
	KindRateLimited          Kind = "rate_limited"
	KindUnavailable          Kind = "unavailable"
	KindInternal             Kind = "internal"
)

// Classified is implemented by domain errors that know their problem kind.
type Classified interface {
	error
	ProblemKind() Kind
}

// Pointed is optionally implemented by errors that point at a request field (JSON Pointer).
type Pointed interface {
	ProblemPath() string
}

// Itemised is optionally implemented by errors carrying several field errors.
type Itemised interface {
	FieldErrors() []FieldError
}

// Versioned is optionally implemented by precondition errors carrying the current ETag.
type Versioned interface {
	CurrentETag() string
}

// Coded is optionally implemented by errors carrying a machine-readable reason code (e.g. last_admin).
type Coded interface {
	ProblemCode() string
}

// PermissionDenied is optionally implemented by 403 errors naming the missing permission.
type PermissionDenied interface {
	MissingPermission() string
}

// Retryable is optionally implemented by 429/503 errors telling the client when to retry (whole seconds).
type Retryable interface {
	RetryAfterSeconds() int
}

// Error is the general-purpose classified error features may return.
type Error struct {
	Kind   Kind
	Detail string
	Path   string
	Cause  error
	origin *Error
}

// NewError builds a classified error with a user-facing detail ("what happened, why, what to do").
func NewError(kind Kind, detail string) *Error {
	return &Error{Kind: kind, Detail: detail}
}

// At returns a copy pointing at a request field; the copy still matches e with errors.Is.
func (e *Error) At(path string) *Error {
	c := e.derive()
	c.Path = path
	return c
}

// Wrap returns a copy that keeps the underlying cause; the copy still matches e with errors.Is.
func (e *Error) Wrap(cause error) *Error {
	c := e.derive()
	c.Cause = cause
	return c
}

// Is matches the sentinel a copy was derived from.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && (t == e || (e.origin != nil && t == e.origin))
}

func (e *Error) derive() *Error {
	c := *e
	if c.origin == nil {
		c.origin = e
	}
	return &c
}

func (e *Error) Error() string       { return string(e.Kind) + ": " + e.Detail }
func (e *Error) Unwrap() error       { return e.Cause }
func (e *Error) ProblemKind() Kind   { return e.Kind }
func (e *Error) ProblemPath() string { return e.Path }

// Entry is the catalog's type and status for one kind.
type Entry struct {
	Type   TypeURI
	Status int
}

// Catalog is the single place that maps error kinds to problem documents.
type Catalog struct {
	entries map[Kind]Entry
}

// GenericInternalDetail is shown for unclassified errors; internals are never leaked.
type GenericInternalDetail string

const InternalDetail GenericInternalDetail = "Something went wrong on our side. Retry, and quote the request id if it persists."

// NewCatalog returns the standard kind → (type, status) mapping (design §4.2).
func NewCatalog() *Catalog {
	return &Catalog{entries: map[Kind]Entry{
		KindValidation:           {TypeValidation, http.StatusUnprocessableEntity},
		KindMalformedBody:        {TypeMalformedBody, http.StatusBadRequest},
		KindUnauthenticated:      {TypeUnauthenticated, http.StatusUnauthorized},
		KindForbidden:            {TypeForbidden, http.StatusForbidden},
		KindNotFound:             {TypeNotFound, http.StatusNotFound},
		KindConflict:             {TypeConflict, http.StatusConflict},
		KindPreconditionFailed:   {TypePreconditionFailed, http.StatusPreconditionFailed},
		KindPayloadTooLarge:      {TypePayloadTooLarge, http.StatusRequestEntityTooLarge},
		KindPreconditionRequired: {TypePreconditionRequired, http.StatusPreconditionRequired},
		KindRateLimited:          {TypeRateLimited, http.StatusTooManyRequests},
		KindUnavailable:          {TypeUnavailable, http.StatusServiceUnavailable},
		KindInternal:             {TypeInternal, http.StatusInternalServerError},
	}}
}

// Register adds or overrides a kind (e.g. features contributing new problem types).
func (c *Catalog) Register(kind Kind, e Entry) {
	c.entries[kind] = e
}

// Resolve converts any error into a problem document; unclassified errors become a generic 500.
func (c *Catalog) Resolve(err error) Details {
	var classified Classified
	if !errors.As(err, &classified) {
		return c.details(KindInternal, string(InternalDetail))
	}
	d := c.details(classified.ProblemKind(), c.detailOf(classified))
	return c.enrich(d, err)
}

func (c *Catalog) details(kind Kind, detail string) Details {
	e, ok := c.entries[kind]
	if !ok {
		e = c.entries[KindInternal]
	}
	return New(e.Type, e.Status, detail)
}

func (c *Catalog) detailOf(err Classified) string {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Detail
	}
	return err.Error()
}

func (c *Catalog) enrich(d Details, err error) Details {
	var items Itemised
	if errors.As(err, &items) {
		d = d.WithErrors(items.FieldErrors())
	}
	var pointed Pointed
	if errors.As(err, &pointed) && pointed.ProblemPath() != "" {
		d.Path = pointed.ProblemPath()
	}
	var versioned Versioned
	if errors.As(err, &versioned) {
		d.CurrentETag = versioned.CurrentETag()
	}
	return c.extend(d, err)
}

func (c *Catalog) extend(d Details, err error) Details {
	var coded Coded
	if errors.As(err, &coded) {
		d.Code = coded.ProblemCode()
	}
	var denied PermissionDenied
	if errors.As(err, &denied) {
		d.MissingPermission = denied.MissingPermission()
	}
	var retryable Retryable
	if errors.As(err, &retryable) {
		d.RetryAfter = retryable.RetryAfterSeconds()
	}
	return d
}
