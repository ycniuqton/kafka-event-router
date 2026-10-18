package testcases

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/config"
)

// TargetSource builds the document a run of one owner kind compiles (base of every owner kind).
type TargetSource interface {
	Target(ctx context.Context, id uuid.UUID, req RunRequest) (RunTarget, error)
}

// RouteTargets runs a route's cases against the request's config or its stored draft with new steps.
type RouteTargets struct {
	Routes RouteSource
}

// Target implements TargetSource.
func (t RouteTargets) Target(ctx context.Context, id uuid.UUID, req RunRequest) (RunTarget, error) {
	info, err := t.Routes.Route(ctx, id)
	if err != nil {
		return RunTarget{}, err
	}
	doc, err := DraftResolver{}.Document(req, info)
	if err != nil {
		return RunTarget{}, err
	}
	return RunTarget{TeamID: info.TeamID, RouteID: id.String(), RouteName: info.Name, Document: doc}, nil
}

// TransformInfo is what a run needs about a library transform: owner team, name and latest steps.
type TransformInfo struct {
	TeamID string
	Name   string
	Steps  json.RawMessage
}

// TransformSource reads library transforms (Postgres in production).
type TransformSource interface {
	Transform(ctx context.Context, id uuid.UUID) (TransformInfo, error)
}

// TransformTargets runs a transform's cases against the request's steps (else its latest version)
// inside a one-branch stand-alone route.
type TransformTargets struct {
	Transforms TransformSource
}

// Target implements TargetSource.
func (t TransformTargets) Target(ctx context.Context, id uuid.UUID, req RunRequest) (RunTarget, error) {
	info, err := t.Transforms.Transform(ctx, id)
	if err != nil {
		return RunTarget{}, err
	}
	steps := req.Steps
	if len(steps) == 0 || string(steps) == nullJSON {
		steps = info.Steps
	}
	doc, err := StepsDocument{}.Of(info.Name, steps)
	if err != nil {
		return RunTarget{}, api.NewValidationError().Add(string(FieldSteps), string(CodeInvalid), string(MsgDraftInvalid))
	}
	return RunTarget{TeamID: info.TeamID, RouteID: id.String(), RouteName: info.Name, Document: doc}, nil
}

// StandaloneRoute names the placeholder source/destination of a transform's test route.
type StandaloneRoute string

const StandaloneRef StandaloneRoute = "library"

// StepsDocument wraps steps into a one-branch route document (branch "main").
type StepsDocument struct{}

// Of returns the route document named name.
func (StepsDocument) Of(name string, steps json.RawMessage) (json.RawMessage, error) {
	var list config.Steps
	if err := json.Unmarshal(steps, &list); err != nil {
		return nil, err
	}
	ref := string(StandaloneRef)
	doc := config.RouteConfig{APIVersion: config.APIVersionRouterV1, Name: name,
		Source: config.Source{ConnectionID: ref, Topics: []string{ref}, Format: config.Format{Type: config.FormatTypeJSON}},
		Branches: []config.Branch{{ID: string(DefaultBranchID), Transform: &config.Transform{Steps: list},
			Destination: config.Destination{ConnectionID: ref, Topic: ref}}},
	}
	return json.Marshal(doc)
}
