package testcaseshttp

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/internal/platform/api"
)

// ListHandler serves GET /api/v1/{routes|transforms}/{id}/test-cases.
type ListHandler struct {
	d Deps
	o OwnerRoutes
}

// Route implements api.Handler.
func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.o.Collection), Permission: h.o.Read, Object: h.d.OwnerObject(h.o)}
}

// Serve implements api.Handler.
func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	owner, err := Params{Kind: h.o.Kind}.Owner(r)
	if err != nil {
		return err
	}
	team, err := h.d.Owners.TeamOf(r.Context(), owner.ID.String())
	if err != nil {
		return err
	}
	cases, err := h.d.Service.List(r.Context(), owner)
	if err != nil {
		return err
	}
	mask := h.d.Masking.For(r, team)
	out := ListDTO{Items: make([]CaseDTO, 0, len(cases))}
	for _, c := range cases {
		out.Items = append(out.Items, mask.Case(Presenter{}.Case(c)))
	}
	return h.d.Responder.JSON(w, http.StatusOK, out)
}

// CreateHandler serves POST /api/v1/{routes|transforms}/{id}/test-cases (pin an event).
type CreateHandler struct {
	d Deps
	o OwnerRoutes
}

// Route implements api.Handler.
func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(h.o.Collection), Permission: h.o.Write, Object: h.d.OwnerObject(h.o),
		Mutating: true, AuditAction: ActionCreated, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	owner, err := Params{Kind: h.o.Kind}.Owner(r)
	if err != nil {
		return err
	}
	team, err := h.d.Owners.TeamOf(r.Context(), owner.ID.String())
	if err != nil {
		return err
	}
	in, err := api.Decoder[CreateInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	c, err := h.d.Service.Create(r.Context(), owner, testcases.NewCase{BranchID: in.BranchID, Name: in.Name, InputEvent: in.InputEvent,
		ExpectedOutcome: in.ExpectedOutcome, ExpectedOutput: in.ExpectedOutput, Author: auth.PrincipalFrom(r.Context()).UserID})
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, ActionCreated, c, team); err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, c.RowVersion, h.d.Masking.For(r, team).Case(Presenter{}.Case(c)))
}

// PatchHandler serves PATCH /api/v1/{routes|transforms}/{id}/test-cases/{caseId} (If-Match).
type PatchHandler struct {
	d Deps
	o OwnerRoutes
}

// Route implements api.Handler.
func (h *PatchHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(h.o.Item), Permission: h.o.Write, Object: h.d.OwnerObject(h.o),
		Mutating: true, AuditAction: ActionUpdated, Versioned: true, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *PatchHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	owner, id, team, err := ItemRequest{d: h.d, o: h.o}.Resolve(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	in, err := api.Decoder[PatchInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	patch := testcases.CasePatch{Name: in.Name, ExpectedOutcome: in.ExpectedOutcome, ExpectedOutput: in.ExpectedOutput}
	c, err := h.d.Service.Update(r.Context(), owner, id, patch, expected)
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, ActionUpdated, c, team); err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, c.RowVersion, h.d.Masking.For(r, team).Case(Presenter{}.Case(c)))
}

// DeleteHandler serves DELETE /api/v1/{routes|transforms}/{id}/test-cases/{caseId}.
type DeleteHandler struct {
	d Deps
	o OwnerRoutes
}

// Route implements api.Handler.
func (h *DeleteHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(h.o.Item), Permission: h.o.Write, Object: h.d.OwnerObject(h.o),
		Mutating: true, AuditAction: ActionDeleted}
}

// Serve implements api.Handler.
func (h *DeleteHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	owner, id, team, err := ItemRequest{d: h.d, o: h.o}.Resolve(r)
	if err != nil {
		return err
	}
	c, err := h.d.Service.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	if err := h.d.Service.Delete(r.Context(), owner, id); err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, ActionDeleted, c, team); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// RunHandler serves POST /api/v1/{routes|transforms}/{id}/test-cases/run {config | steps, caseIds?}.
type RunHandler struct {
	d Deps
	o OwnerRoutes
}

// Route implements api.Handler (a run changes nothing: read permission).
func (h *RunHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(h.o.Run), Permission: h.o.Read, Object: h.d.OwnerObject(h.o),
		BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *RunHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	owner, err := Params{Kind: h.o.Kind}.Owner(r)
	if err != nil {
		return err
	}
	team, err := h.d.Owners.TeamOf(r.Context(), owner.ID.String())
	if err != nil {
		return err
	}
	in, err := api.Decoder[RunInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	rep, err := h.d.Service.Run(r.Context(), owner, testcases.RunRequest{Config: in.Config, Steps: in.Steps, CaseIDs: in.CaseIDs})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, h.d.Masking.For(r, team).Report(Presenter{}.Report(rep)))
}

// ItemRequest resolves the owner, case id and owner team of /test-cases/{caseId} requests.
type ItemRequest struct {
	d Deps
	o OwnerRoutes
}

// Resolve returns owner, case id and team (404 for unknown routes or malformed ids).
func (q ItemRequest) Resolve(r *http.Request) (testcases.Owner, uuid.UUID, string, error) {
	owner, err := Params{Kind: q.o.Kind}.Owner(r)
	if err != nil {
		return testcases.Owner{}, uuid.Nil, "", err
	}
	id, err := Params{}.CaseID(r)
	if err != nil {
		return testcases.Owner{}, uuid.Nil, "", err
	}
	team, err := q.d.Owners.TeamOf(r.Context(), owner.ID.String())
	return owner, id, team, err
}
