// Package metricshttp serves the monitoring endpoints: dashboard summary and attention, recent
// activity, the first-run checklist and route overview metrics (class-based handlers).
package metricshttp

import (
	"context"
	"net/http"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/dashboard"
	"github.com/eventrouter/router/internal/metrics/flowmap"
	"github.com/eventrouter/router/internal/metrics/overview"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
	"github.com/eventrouter/router/internal/store"
)

// Path names the monitoring endpoints.
type Path string

const (
	PathSummary      Path = "/api/v1/dashboard/summary"
	PathAttention    Path = "/api/v1/dashboard/attention"
	PathActivity     Path = "/api/v1/activity"
	PathOnboarding   Path = "/api/v1/onboarding"
	PathRouteMetrics Path = "/api/v1/routes/{id}/metrics"
	PathFlowMap      Path = "/api/v1/flow-map"
)

// FlowMapParam names the GET /flow-map filters.
type FlowMapParam string

const (
	ParamConnection FlowMapParam = "connection"
	ParamTeam       FlowMapParam = "team"
	ParamTag        FlowMapParam = "tag"
)

// Permissions of the monitoring endpoints (keys of access-control's permission catalog).
var (
	PermMetricsRead = auth.Permission{Key: "metrics.read", Access: auth.AccessRead}
	PermRouteRead   = routeshttp.PermRead
)

// SummaryBuilder builds the health summary.
type SummaryBuilder interface {
	Build(ctx context.Context) (dashboard.Summary, error)
}

// AttentionDetector returns the needs-attention rows.
type AttentionDetector interface {
	Detect(ctx context.Context) ([]dashboard.AttentionItem, error)
}

// ActivityLister returns route-change pages.
type ActivityLister interface {
	List(ctx context.Context, limit int, after *store.Cursor) (dashboard.ActivityPage, error)
}

// OnboardingBuilder returns the first-run checklist.
type OnboardingBuilder interface {
	Build(ctx context.Context) (dashboard.Onboarding, error)
}

// OverviewLoader returns one route's overview metrics.
type OverviewLoader interface {
	Load(ctx context.Context, id routes.RouteID, r metrics.TimeRange) (overview.RouteMetrics, error)
}

// FlowMapBuilder returns the dashboard flow map.
type FlowMapBuilder interface {
	Build(ctx context.Context, f flowmap.Filters) (flowmap.FlowMap, error)
}

// Deps are the services and conventions the handlers use.
type Deps struct {
	FlowMap    FlowMapBuilder
	Summary    SummaryBuilder
	Attention  AttentionDetector
	Activity   ActivityLister
	Onboarding OnboardingBuilder
	Overview   OverviewLoader
	Demo       DemoSeeder
	Responder  *api.Responder
	Pages      *api.PageParser
}

// NewModule returns every monitoring handler.
func NewModule(d Deps) api.Module {
	return api.Modules{&SummaryHandler{d: d}, &AttentionHandler{d: d}, &ActivityHandler{d: d}, &OnboardingHandler{d: d},
		&RouteMetricsHandler{d: d}, &FlowMapHandler{d: d}, &DemoCreateHandler{d: d}, &DemoDeleteHandler{d: d}}
}

// FlowMapHandler serves GET /flow-map?connection=&team=&tag=.
type FlowMapHandler struct{ d Deps }

func (h *FlowMapHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathFlowMap), Permission: PermMetricsRead}
}

func (h *FlowMapHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	m, err := h.d.FlowMap.Build(r.Context(), flowmap.Filters{Connection: q.Get(string(ParamConnection)),
		Team: q.Get(string(ParamTeam)), Tag: q.Get(string(ParamTag))})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, m)
}

// SummaryHandler serves GET /dashboard/summary.
type SummaryHandler struct{ d Deps }

func (h *SummaryHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathSummary), Permission: PermMetricsRead}
}

func (h *SummaryHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	s, err := h.d.Summary.Build(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, s)
}

// AttentionBody is the GET /dashboard/attention response.
type AttentionBody struct {
	Items []dashboard.AttentionItem `json:"items"`
}

// AttentionHandler serves GET /dashboard/attention.
type AttentionHandler struct{ d Deps }

func (h *AttentionHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathAttention), Permission: PermMetricsRead}
}

func (h *AttentionHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	items, err := h.d.Attention.Detect(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, AttentionBody{Items: items})
}

// ActivityHandler serves GET /activity?limit=10&cursor= (route changes from the audit log).
type ActivityHandler struct{ d Deps }

func (h *ActivityHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathActivity), Permission: PermRouteRead}
}

func (h *ActivityHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	if !r.URL.Query().Has(string(api.ParamLimit)) {
		page.Limit = int(dashboard.DefaultActivityLimit)
	}
	res, err := h.d.Activity.List(r.Context(), page.Limit, page.After)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[dashboard.ActivityItem]{Items: res.Items, NextCursor: h.d.Pages.Next(res.Next)})
}

// OnboardingHandler serves GET /onboarding.
type OnboardingHandler struct{ d Deps }

func (h *OnboardingHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathOnboarding), Permission: PermMetricsRead}
}

func (h *OnboardingHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	o, err := h.d.Onboarding.Build(r.Context())
	if err != nil {
		return err
	}
	if o, err = (OnboardingDemo{d: h.d}).Apply(r.Context(), o); err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, o)
}

// RouteMetricsHandler serves GET /routes/{id}/metrics?range=15m|1h|24h|7d.
type RouteMetricsHandler struct{ d Deps }

func (h *RouteMetricsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteMetrics), Permission: PermMetricsRead, Object: routeshttp.RouteObject}
}

func (h *RouteMetricsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	rng, err := metrics.ParseRange(r.URL.Query().Get(string(metrics.ParamRange)))
	if err != nil {
		return err
	}
	id, err := routeshttp.Params{}.RouteID(r)
	if err != nil {
		return err
	}
	m, err := h.d.Overview.Load(r.Context(), id, rng)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, m)
}
