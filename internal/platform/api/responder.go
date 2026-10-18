package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/platform/requestid"
	"github.com/eventrouter/router/internal/store"
)

// Header names response/request headers owned by the API conventions.
type Header string

const (
	HeaderETag         Header = "ETag"
	HeaderIfMatch      Header = "If-Match"
	HeaderCacheControl Header = "Cache-Control"
	HeaderLocation     Header = "Location"
)

// CacheDirective names Cache-Control values.
type CacheDirective string

const CacheNoStore CacheDirective = "no-store"

// StatusServerErrorMin is the first status logged as a server error.
type StatusClass int

const StatusServerErrorMin StatusClass = 500

// Responder writes JSON bodies and maps errors to problem documents through the Catalog.
type Responder struct {
	catalog *problem.Catalog
	writer  problem.Writer
	log     *slog.Logger
}

// NewResponder builds a responder.
func NewResponder(catalog *problem.Catalog, log *slog.Logger) *Responder {
	return &Responder{catalog: catalog, writer: problem.Writer{RequestID: RequestIDOf}, log: log}
}

// JSON writes body with status.
func (rs *Responder) JSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}

// Versioned writes body with a strong ETag from its row version.
func (rs *Responder) Versioned(w http.ResponseWriter, status int, version store.RowVersion, body any) error {
	w.Header().Set(string(HeaderETag), version.ETag())
	return rs.JSON(w, status, body)
}

// NoContent writes 204.
func (rs *Responder) NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// LogMessage names the responder's log lines.
type LogMessage string

const (
	LogRequestFailed    LogMessage = "request failed"
	LogRequestAbandoned LogMessage = "request abandoned by client"
)

// Problem converts err into an RFC 7807 response; 5xx causes are logged, never shown.
func (rs *Responder) Problem(w http.ResponseWriter, r *http.Request, err error) {
	d := rs.catalog.Resolve(err)
	if d.Status >= int(StatusServerErrorMin) {
		rs.logServerError(r, err)
	}
	rs.writer.Write(w, r, d)
}

// logServerError logs a 5xx cause as an error, or only at debug level when the client
// already abandoned the request (its cancellation surfaces as a downstream failure).
func (rs *Responder) logServerError(r *http.Request, err error) {
	attrs := []any{string(logging.AttrRequestID), RequestIDOf(r), string(logging.AttrError), err}
	if r.Context().Err() != nil {
		rs.log.Debug(string(LogRequestAbandoned), attrs...)
		return
	}
	rs.log.Error(string(LogRequestFailed), attrs...)
}

// Writer exposes the problem writer for router-level fallbacks (404/405).
func (rs *Responder) Writer() problem.Writer {
	return rs.writer
}

// RequestIDOf returns the request id assigned by RequestIDMiddleware.
func RequestIDOf(r *http.Request) string {
	return requestid.From(r.Context())
}
