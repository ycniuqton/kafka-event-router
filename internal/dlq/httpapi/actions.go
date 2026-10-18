package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/platform/api"
)

// MatchBody is the filter of a match selection.
type MatchBody struct {
	ErrorType       []string   `json:"errorType"`
	Since           *time.Time `json:"since"`
	Until           *time.Time `json:"until"`
	Step            string     `json:"step"`
	Version         int        `json:"version"`
	Text            string     `json:"q"`
	IncludeResolved bool       `json:"includeResolved"`
}

// SelectionBody is the body of retry and discard (exactly one of ids / match).
type SelectionBody struct {
	IDs           []string   `json:"ids"`
	Match         *MatchBody `json:"match"`
	ExpectedCount *int       `json:"expectedCount"`
}

// RequestValidator converts bodies into domain selections.
type RequestValidator struct{}

// Selection validates the shape and the filter values.
func (v RequestValidator) Selection(b SelectionBody) (dlq.Selection, error) {
	sel := dlq.Selection{IDs: b.IDs, ExpectedCount: b.ExpectedCount}
	if b.Match != nil {
		q, err := v.Match(*b.Match)
		if err != nil {
			return dlq.Selection{}, err
		}
		sel.Match = &q
	}
	return sel, sel.Validate()
}

// Match converts a body filter into a query.
func (RequestValidator) Match(m MatchBody) (dlq.Query, error) {
	q := dlq.Query{Step: m.Step, Text: m.Text, IncludeResolved: m.IncludeResolved}
	for i, t := range m.ErrorType {
		if !record.ErrorType(t).Valid() {
			return dlq.Query{}, dlq.ErrInvalidFilter.At(PathMatchErrType.Indexed(i))
		}
		q.ErrorTypes = append(q.ErrorTypes, record.ErrorType(t))
	}
	if m.Version < 0 {
		return dlq.Query{}, dlq.ErrInvalidFilter.At(string(dlq.PathMatchVer))
	}
	q.VersionNo = m.Version
	q.Since, q.Until = (TimeOf{}).Value(m.Since), (TimeOf{}).Value(m.Until)
	return q, nil
}

// PathMatchErrType points at body match error types.
const PathMatchErrType dlq.RequestPath = "/match/errorType"

// TimeOf dereferences optional times.
type TimeOf struct{}

// Value returns *t or the zero time.
func (TimeOf) Value(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// RetryHandler serves POST /routes/{id}/dlq/retry.
type RetryHandler struct{ d Deps }

func (h *RetryHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathRetry), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: audit.ActionDLQRetried}
}

func (h *RetryHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	report, err := ActionCall{d: h.d}.Run(w, r, dlq.ActionRetry)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ReportDTOs{}.Retry(report))
}

// DiscardHandler serves POST /routes/{id}/dlq/discard.
type DiscardHandler struct{ d Deps }

func (h *DiscardHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDiscard), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: audit.ActionDLQDiscarded}
}

func (h *DiscardHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	report, err := ActionCall{d: h.d}.Run(w, r, dlq.ActionDiscard)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ReportDTOs{}.Discard(report))
}

// RestoreHandler serves POST /routes/{id}/dlq/restore (un-resolve discarded events, P2).
type RestoreHandler struct{ d Deps }

func (h *RestoreHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathRestore), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: dlq.ActionDLQRestored}
}

func (h *RestoreHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	report, err := ActionCall{d: h.d}.Run(w, r, dlq.ActionRestore)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ReportDTOs{}.Restore(report))
}

// ActionCall is the shared decode → validate → Manager.Act step of action handlers.
type ActionCall struct{ d Deps }

// Run decodes the selection and applies kind.
func (c ActionCall) Run(w http.ResponseWriter, r *http.Request, kind dlq.ActionKind) (dlq.ActionReport, error) {
	body, err := api.Decoder[SelectionBody]{}.Decode(w, r)
	if err != nil {
		return dlq.ActionReport{}, err
	}
	sel, err := RequestValidator{}.Selection(body)
	if err != nil {
		return dlq.ActionReport{}, err
	}
	return c.d.Manager.Act(r.Context(), kind, Caller{}.Actor(r), chi.URLParam(r, string(ParamRouteID)), sel)
}
