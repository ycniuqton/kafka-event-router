// Package alertshttp serves /alert-rules (per-route rules, test button) and /alerts (bell feed, ack).
package alertshttp

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// Path names the alerting endpoints.
type Path string

const (
	PathRules    Path = "/api/v1/alert-rules"
	PathRule     Path = "/api/v1/alert-rules/{id}"
	PathRuleTest Path = "/api/v1/alert-rules/{id}/test"
	PathFeed     Path = "/api/v1/alerts"
	PathAck      Path = "/api/v1/alerts/{id}/ack"
)

// Param names query and path parameters.
type Param string

const (
	ParamID      Param = "id"
	ParamRouteID Param = "routeId"
	ParamState   Param = "state"
)

// Permissions of the alerting endpoints (access-control catalog keys).
var (
	PermAlertRead  = auth.Permission{Key: "alert.read", Access: auth.AccessRead}
	PermAlertWrite = auth.Permission{Key: "alert.write", Access: auth.AccessWrite}
)

// ErrBadRouteID rejects a malformed routeId filter.
var ErrBadRouteID = problem.NewError(problem.KindMalformedBody, "routeId must be a route id.").At(string(alerts.PathRouteID))

// RuleManager is the rule use-case port.
type RuleManager interface {
	List(ctx context.Context, routeID *uuid.UUID) ([]alerts.AlertRule, error)
	Create(ctx context.Context, actor *uuid.UUID, d alerts.RuleDraft) (alerts.AlertRule, error)
	Patch(ctx context.Context, id alerts.RuleID, expected store.RowVersion, p alerts.RulePatch) (alerts.AlertRule, error)
	Delete(ctx context.Context, id alerts.RuleID) error
	Test(ctx context.Context, id alerts.RuleID) ([]notify.ChannelResult, error)
}

// FeedManager is the bell use-case port.
type FeedManager interface {
	List(ctx context.Context, user uuid.UUID, state *alerts.AlertState, limit int, after *store.Cursor) (alerts.FeedPage, error)
	Ack(ctx context.Context, id alerts.EventID, user uuid.UUID) error
}

// Deps are the handlers' collaborators.
type Deps struct {
	Rules     RuleManager
	Feed      FeedManager
	Responder *api.Responder
	Pages     *api.PageParser
}

// NewModule returns every alerting handler.
func NewModule(d Deps) api.Module {
	return api.Modules{&ListRulesHandler{d: d}, &CreateRuleHandler{d: d}, &PatchRuleHandler{d: d}, &DeleteRuleHandler{d: d},
		&TestRuleHandler{d: d}, &FeedHandler{d: d}, &AckHandler{d: d}}
}

// Params reads typed parameters.
type Params struct{}

// RuleID returns {id}; malformed ids are unknown rules (404).
func (Params) RuleID(r *http.Request) (alerts.RuleID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, alerts.ErrRuleNotFound
	}
	return id, nil
}

// EventID returns {id} of an alert.
func (Params) EventID(r *http.Request) (alerts.EventID, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, string(ParamID)), 10, 64)
	if err != nil {
		return 0, alerts.ErrAlertNotFound
	}
	return alerts.EventID(id), nil
}

// RouteFilter returns the optional ?routeId.
func (Params) RouteFilter(r *http.Request) (*uuid.UUID, error) {
	raw := r.URL.Query().Get(string(ParamRouteID))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, ErrBadRouteID
	}
	return &id, nil
}

// Actor returns the caller's user id, if any.
func (Params) Actor(r *http.Request) *uuid.UUID {
	p := auth.PrincipalFrom(r.Context())
	if p.UserID == uuid.Nil {
		return nil
	}
	return &p.UserID
}

// RulesBody is the GET /alert-rules response.
type RulesBody struct {
	Items []alerts.RuleView `json:"items"`
}

// TestBody is the POST /alert-rules/{id}/test response.
type TestBody struct {
	Results []notify.ChannelResult `json:"results"`
}

// FeedBody is the GET /alerts response.
type FeedBody struct {
	Items      []alerts.FeedItem `json:"items"`
	NextCursor *string           `json:"next_cursor"`
	Unread     int               `json:"unread"`
}

// ListRulesHandler serves GET /alert-rules?routeId=.
type ListRulesHandler struct{ d Deps }

func (h *ListRulesHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRules), Permission: PermAlertRead}
}

func (h *ListRulesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	route, err := Params{}.RouteFilter(r)
	if err != nil {
		return err
	}
	rules, err := h.d.Rules.List(r.Context(), route)
	if err != nil {
		return err
	}
	views := make([]alerts.RuleView, 0, len(rules))
	for _, rule := range rules {
		views = append(views, rule.View())
	}
	return h.d.Responder.JSON(w, http.StatusOK, RulesBody{Items: views})
}

// CreateRuleHandler serves POST /alert-rules.
type CreateRuleHandler struct{ d Deps }

func (h *CreateRuleHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathRules), Permission: PermAlertWrite, Mutating: true,
		AuditAction: audit.ActionAlertRuleSaved}
}

func (h *CreateRuleHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[alerts.RuleDraft]{}.Decode(w, r)
	if err != nil {
		return err
	}
	rule, err := h.d.Rules.Create(r.Context(), Params{}.Actor(r), body)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, rule.RowVersion, rule.View())
}

// PatchRuleHandler serves PATCH /alert-rules/{id} with If-Match.
type PatchRuleHandler struct{ d Deps }

func (h *PatchRuleHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathRule), Permission: PermAlertWrite, Mutating: true,
		AuditAction: audit.ActionAlertRuleSaved, Versioned: true}
}

func (h *PatchRuleHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RuleID(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[alerts.RulePatch]{}.Decode(w, r)
	if err != nil {
		return err
	}
	rule, err := h.d.Rules.Patch(r.Context(), id, expected, body)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, rule.RowVersion, rule.View())
}

// DeleteRuleHandler serves DELETE /alert-rules/{id}.
type DeleteRuleHandler struct{ d Deps }

func (h *DeleteRuleHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathRule), Permission: PermAlertWrite, Mutating: true,
		AuditAction: audit.ActionAlertRuleSaved}
}

func (h *DeleteRuleHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RuleID(r)
	if err != nil {
		return err
	}
	if err := h.d.Rules.Delete(r.Context(), id); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// TestRuleHandler serves POST /alert-rules/{id}/test.
type TestRuleHandler struct{ d Deps }

func (h *TestRuleHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathRuleTest), Permission: PermAlertWrite}
}

func (h *TestRuleHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RuleID(r)
	if err != nil {
		return err
	}
	results, err := h.d.Rules.Test(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, TestBody{Results: results})
}

// FeedHandler serves GET /alerts?state=&limit=&cursor=.
type FeedHandler struct{ d Deps }

func (h *FeedHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathFeed), Permission: PermAlertRead}
}

func (h *FeedHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	var state *alerts.AlertState
	if raw := r.URL.Query().Get(string(ParamState)); raw != "" {
		s := alerts.AlertState(raw)
		state = &s
	}
	user := auth.PrincipalFrom(r.Context()).UserID
	res, err := h.d.Feed.List(r.Context(), user, state, page.Limit, page.After)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, FeedBody{Items: res.Items, NextCursor: h.d.Pages.Next(res.Next), Unread: res.Unread})
}

// AckHandler serves POST /alerts/{id}/ack (the caller's own read state; not audited).
type AckHandler struct{ d Deps }

func (h *AckHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathAck), Permission: PermAlertRead}
}

func (h *AckHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.EventID(r)
	if err != nil {
		return err
	}
	if err := h.d.Feed.Ack(r.Context(), id, auth.PrincipalFrom(r.Context()).UserID); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}
