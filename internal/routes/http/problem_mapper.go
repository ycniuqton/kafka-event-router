package routeshttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/pkg/config"
)

// IssuesProblem is a 422 problem document with the full issue list as an extension member.
type IssuesProblem struct {
	problem.Details
	Issues config.Issues `json:"issues"`
}

// IssuesProblemWriter renders routes.InvalidConfigError with `issues`; other errors pass through.
type IssuesProblemWriter struct {
	Responder *api.Responder
}

// Wrap decorates h.
func (p IssuesProblemWriter) Wrap(h api.Handler) api.Handler {
	return issuesHandler{inner: h, writer: p}
}

// Write renders the problem when err carries issues and reports whether it did.
func (p IssuesProblemWriter) Write(w http.ResponseWriter, r *http.Request, err error) bool {
	var invalid *routes.InvalidConfigError
	if !errors.As(err, &invalid) {
		return false
	}
	d := problem.New(problem.TypeValidation, http.StatusUnprocessableEntity, invalid.Error()).WithErrors(invalid.FieldErrors())
	d.Instance, d.RequestID = r.URL.Path, api.RequestIDOf(r)
	w.Header().Set(string(problem.HeaderContentType), string(problem.MediaTypeProblemJSON))
	w.WriteHeader(d.Status)
	_ = json.NewEncoder(w).Encode(IssuesProblem{Details: d, Issues: invalid.Issues})
	return true
}

// issuesHandler is a decorated handler.
type issuesHandler struct {
	inner  api.Handler
	writer IssuesProblemWriter
}

func (h issuesHandler) Route() api.RouteSpec { return h.inner.Route() }

func (h issuesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	err := h.inner.Serve(w, r)
	if err != nil && h.writer.Write(w, r, err) {
		return nil
	}
	return err
}
