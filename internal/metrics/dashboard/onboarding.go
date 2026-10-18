package dashboard

import "context"

// OnboardingStepID names a first-run checklist step.
type OnboardingStepID string

const (
	StepConnection OnboardingStepID = "connection"
	StepRoute      OnboardingStepID = "route"
	StepDeploy     OnboardingStepID = "deploy"
)

// OnboardingHref is the web path each step links to.
type OnboardingHref string

const (
	HrefConnections OnboardingHref = "/connections"
	HrefNewRoute    OnboardingHref = "/routes/new"
	HrefRoutes      OnboardingHref = "/routes"
)

// OnboardingStep is one checklist row.
type OnboardingStep struct {
	ID   OnboardingStepID `json:"id"`
	Done bool             `json:"done"`
	Href OnboardingHref   `json:"href"`
}

// Onboarding is GET /onboarding; demoAvailable says whether "Try with demo data" is offered and demo is
// the caller's team demo connection, if seeded (P3).
type Onboarding struct {
	Steps         []OnboardingStep `json:"steps"`
	AllDone       bool             `json:"allDone"`
	DemoAvailable bool             `json:"demoAvailable"`
	Demo          *DemoSeed        `json:"demo"`
}

// OnboardingService derives the checklist from counts.
type OnboardingService struct {
	Counts OnboardingCounter
}

// Build returns the checklist.
func (s OnboardingService) Build(ctx context.Context) (Onboarding, error) {
	c, err := s.Counts.OnboardingCounts(ctx)
	if err != nil {
		return Onboarding{}, err
	}
	steps := []OnboardingStep{
		{ID: StepConnection, Done: c.Connections > 0, Href: HrefConnections},
		{ID: StepRoute, Done: c.Routes > 0, Href: HrefNewRoute},
		{ID: StepDeploy, Done: c.ActiveDeployments > 0, Href: HrefRoutes},
	}
	all := true
	for _, st := range steps {
		all = all && st.Done
	}
	return Onboarding{Steps: steps, AllDone: all}, nil
}
