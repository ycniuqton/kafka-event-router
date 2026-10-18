package metricshttp

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/internal/metrics/dashboard"
	"github.com/eventrouter/router/internal/platform/api"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
)

// PathDemo is the demo data endpoint of the first-run checklist (P3).
const PathDemo Path = "/api/v1/onboarding/demo"

// PermDemoWrite is the permission to seed or remove demo data (editor, like creating a route).
var PermDemoWrite = routeshttp.PermWrite

// DemoSeeder seeds and removes a team's demo connection.
type DemoSeeder interface {
	Create(ctx context.Context, team uuid.UUID) (dashboard.DemoSeed, error)
	Remove(ctx context.Context, team uuid.UUID) error
	Status(ctx context.Context, team uuid.UUID) (*dashboard.DemoSeed, error)
}

// DemoTeam picks the team the caller's demo data belongs to: the first team the caller can write to.
type DemoTeam struct{}

// Of returns the caller's demo team (422 when the caller has no team).
func (DemoTeam) Of(ctx context.Context) (uuid.UUID, error) {
	return connhttp.OwnerTeamPolicy{}.Resolve(auth.PrincipalFrom(ctx), nil)
}

// DemoCreateHandler serves POST /onboarding/demo → 201 {connectionId, topic}; 409 when it exists.
type DemoCreateHandler struct{ d Deps }

func (h *DemoCreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDemo), Permission: PermDemoWrite,
		Mutating: true, AuditAction: connections.ActionCreated}
}

func (h *DemoCreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team, err := DemoTeam{}.Of(r.Context())
	if err != nil {
		return err
	}
	seed, err := h.d.Demo.Create(r.Context(), team)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusCreated, seed)
}

// DemoDeleteHandler serves DELETE /onboarding/demo → 204.
type DemoDeleteHandler struct{ d Deps }

func (h *DemoDeleteHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathDemo), Permission: PermDemoWrite,
		Mutating: true, AuditAction: connections.ActionDeleted}
}

func (h *DemoDeleteHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team, err := DemoTeam{}.Of(r.Context())
	if err != nil {
		return err
	}
	if err := h.d.Demo.Remove(r.Context(), team); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// OnboardingDemo adds the caller's demo state to the checklist (no team or no seeder → none offered).
type OnboardingDemo struct{ d Deps }

// Apply sets demoAvailable and demo.
func (o OnboardingDemo) Apply(ctx context.Context, ob dashboard.Onboarding) (dashboard.Onboarding, error) {
	if o.d.Demo == nil {
		return ob, nil
	}
	team, err := DemoTeam{}.Of(ctx)
	if err != nil {
		return ob, nil
	}
	ob.DemoAvailable = true
	ob.Demo, err = o.d.Demo.Status(ctx, team)
	return ob, err
}
