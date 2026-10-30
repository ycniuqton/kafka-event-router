package lookupshttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/lookup"
)

// FilterMessage holds list filter validation texts.
type FilterMessage string

const (
	MsgKindFilter FilterMessage = "Filter by http, redis, postgres or static."
	MsgEnvFilter  FilterMessage = "Filter by dev, staging or prod."
)

// ListHandler serves GET /api/v1/lookup-sources.
type ListHandler struct{ d Deps }

func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathCollection), Permission: PermRead}
}

func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	q, err := ListQueryParser{}.Parse(r, page)
	if err != nil {
		return err
	}
	result, counts, err := h.d.Service.List(r.Context(), q)
	if err != nil {
		return err
	}
	items := make([]SourceDTO, 0, len(result.Items))
	for _, s := range result.Items {
		items = append(items, Presenter{}.Source(s, UsedByDTO{Count: counts[s.ID]}))
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[SourceDTO]{Items: items, NextCursor: h.d.Pages.Next(result.Next)})
}

// ListQueryParser reads the kind/env/q filters.
type ListQueryParser struct{}

// Parse validates the filters.
func (ListQueryParser) Parse(r *http.Request, page api.PageRequest) (lookups.ListQuery, error) {
	v := r.URL.Query()
	q := lookups.ListQuery{Kind: lookup.SourceKind(v.Get(string(QueryKind))), Env: lookups.EnvTag(v.Get(string(QueryEnv))),
		Text: v.Get(string(QueryText)), Limit: page.Limit, After: page.After}
	verr := api.NewValidationError()
	if q.Kind != "" && !q.Kind.Valid() {
		verr.Add("/"+string(QueryKind), string(lookups.CodeInvalid), string(MsgKindFilter))
	}
	if q.Env != "" && !q.Env.Valid() {
		verr.Add("/"+string(QueryEnv), string(lookups.CodeInvalid), string(MsgEnvFilter))
	}
	return q, verr.OrNil()
}

// GetHandler serves GET /api/v1/lookup-sources/{id}.
type GetHandler struct{ d Deps }

func (h *GetHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathItem), Permission: PermRead, Object: h.d.Object}
}

func (h *GetHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	s, routes, err := h.d.Service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, s.Version, Presenter{}.Source(s, UsedByDTO{Count: len(routes), Routes: routes}))
}

// CreateHandler serves POST /api/v1/lookup-sources.
type CreateHandler struct{ d Deps }

func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathCollection), Permission: PermWrite, Mutating: true, AuditAction: lookups.ActionCreated}
}

func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[SourceInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	team, err := OwnerTeam{d: h.d}.Resolve(r.Context(), in.Owner)
	if err != nil {
		return err
	}
	s, err := h.d.Service.Create(r.Context(), in.Draft(team))
	if err != nil {
		return err
	}
	w.Header().Set(string(api.HeaderLocation), string(PathCollection)+"/"+s.ID.String())
	return h.d.Responder.Versioned(w, http.StatusCreated, s.Version, Presenter{}.Source(s, UsedByDTO{}))
}

// UpdateHandler serves PATCH /api/v1/lookup-sources/{id} (JSON merge patch, If-Match required).
type UpdateHandler struct{ d Deps }

func (h *UpdateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathItem), Permission: PermWrite, Object: h.d.Object, Mutating: true,
		AuditAction: lookups.ActionUpdated, Versioned: true}
}

func (h *UpdateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	current, routes, err := h.d.Service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	in, err := MergePatcher{}.Apply(w, r, InputOf(current))
	if err != nil {
		return err
	}
	s, err := h.d.Service.Update(r.Context(), id, expected, in.Draft(current.TeamID))
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, s.Version, Presenter{}.Source(s, UsedByDTO{Count: len(routes), Routes: routes}))
}

// MergePatcher applies an RFC 7396 merge patch body onto the current input.
type MergePatcher struct{}

// Apply returns the merged input, rejecting malformed bodies and unknown fields.
func (MergePatcher) Apply(w http.ResponseWriter, r *http.Request, current SourceInput) (SourceInput, error) {
	patch, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(api.BodyLimitDefault)))
	if err != nil {
		return SourceInput{}, api.JSONErrorMapper{}.Map(err)
	}
	base, err := json.Marshal(current)
	if err != nil {
		return SourceInput{}, err
	}
	merged, err := jsonpatch.MergePatch(base, patch)
	if err != nil {
		return SourceInput{}, api.JSONErrorMapper{}.Map(err)
	}
	var out SourceInput
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return SourceInput{}, api.JSONErrorMapper{}.Map(err)
	}
	return out, nil
}

// DeleteHandler serves DELETE /api/v1/lookup-sources/{id}.
type DeleteHandler struct{ d Deps }

func (h *DeleteHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathItem), Permission: PermWrite, Object: h.d.Object, Mutating: true, AuditAction: lookups.ActionDeleted}
}

func (h *DeleteHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	if err := h.d.Service.Delete(r.Context(), id); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// TestSavedHandler serves POST /api/v1/lookup-sources/{id}/test.
type TestSavedHandler struct{ d Deps }

func (h *TestSavedHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTestSaved), Permission: PermWrite, Object: h.d.Object, Mutating: true, AuditAction: lookups.ActionTested}
}

func (h *TestSavedHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	res, err := h.d.Tests.Saved(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, Presenter{}.Test(res))
}

// TestDraftHandler serves POST /api/v1/lookup-sources/test (unsaved draft).
type TestDraftHandler struct{ d Deps }

func (h *TestDraftHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTestDraft), Permission: PermWrite, Mutating: true, AuditAction: lookups.ActionTested}
}

func (h *TestDraftHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[SourceInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	team, err := OwnerTeam{d: h.d}.Resolve(r.Context(), in.Owner)
	if err != nil {
		return err
	}
	res, err := h.d.Tests.Draft(r.Context(), in.Draft(team))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, Presenter{}.Test(res))
}

// SpecHandler serves GET /internal/v1/lookup-sources/{id}/spec to workloads (decrypted secrets, audited as a secret read).
type SpecHandler struct{ d Deps }

func (h *SpecHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathSpec), Auth: api.AuthRequired}
}

func (h *SpecHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	s, spec, err := h.d.Service.Resolve(r.Context(), id)
	if err != nil {
		return err
	}
	team := s.TeamID
	entry := api.AuditContext(r).Apply(audit.Entry{Action: audit.ActionSecretRead, Object: lookups.ObjectLookupSource, ObjectID: s.ID.String(), TeamID: &team})
	if err := h.d.Recorder.Record(r.Context(), entry); err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, spec)
}
