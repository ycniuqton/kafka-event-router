package http

import (
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
)

// ListAuditLogHandler serves GET /audit-log?actor=&action=&objectType=&objectId=&teamId=&from=&to=&sensitive=&q=&cursor=&limit=.
type ListAuditLogHandler struct {
	d       Deps
	filters *audit.FilterParser
}

func (h *ListAuditLogHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathAuditLog), Permission: PermAuditRead}
}

// Serve validates the filter and page, then returns one keyset page (newest first).
func (h *ListAuditLogHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	filter, err := h.filters.Parse(r.URL.Query())
	if err != nil {
		return err
	}
	res, err := h.d.Audit.Query(r.Context(), filter, audit.Page{Limit: page.Limit, After: page.After})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[audit.LogEntry]{Items: res.Items, NextCursor: h.d.Pages.Next(res.Next)})
}

// ActionInfo is one catalogued action offered by the audit filter bar.
type ActionInfo struct {
	Action    audit.Action   `json:"action"`
	Category  audit.Category `json:"category"`
	Sensitive bool           `json:"sensitive"`
}

// ActionList is the body of GET /audit-log/actions.
type ActionList struct {
	Items []ActionInfo `json:"items"`
}

// ListAuditActionsHandler serves GET /audit-log/actions (the action catalog, sorted).
type ListAuditActionsHandler struct{ d Deps }

func (h *ListAuditActionsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathAuditActions), Permission: PermAuditRead}
}

// Serve lists every action with its category and sensitivity.
func (h *ListAuditActionsHandler) Serve(w http.ResponseWriter, _ *http.Request) error {
	catalog := h.d.Audit.Catalog()
	out := ActionList{Items: []ActionInfo{}}
	for _, a := range catalog.Actions() {
		meta, _ := catalog.Describe(a)
		out.Items = append(out.Items, ActionInfo{Action: a, Category: meta.Category, Sensitive: meta.Sensitive})
	}
	return h.d.Responder.JSON(w, http.StatusOK, out)
}
