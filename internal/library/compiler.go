package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// StepCompiler proves that a library transform's steps compile on the engine for the owner team.
type StepCompiler interface {
	Compile(ctx context.Context, steps json.RawMessage, team uuid.UUID) error
}

// CompileOptionsSource supplies the compile options of a publish check (JS runtime, library resolver).
type CompileOptionsSource interface {
	Options(team uuid.UUID) engine.CompileOptions
}

// StaticCompileOptions is a fixed CompileOptionsSource.
type StaticCompileOptions engine.CompileOptions

// Options implements CompileOptionsSource.
func (o StaticCompileOptions) Options(uuid.UUID) engine.CompileOptions {
	return engine.CompileOptions(o)
}

// CheckRoute names the stand-alone route a transform is compiled in.
type CheckRoute string

const (
	CheckName   CheckRoute = "library-publish-check"
	CheckRef    CheckRoute = "library"
	CheckBranch CheckRoute = "main"
	// CheckStepsPrefix is the pointer of the steps inside the check route (stripped from issue paths).
	CheckStepsPrefix CheckRoute = "/branches/0/transform"
)

// EngineStepCompiler compiles the steps inside a one-branch route document with pkg/engine.
type EngineStepCompiler struct {
	Engine  engine.Compiler
	Options CompileOptionsSource
}

// Compile returns a 422 whose paths point at /steps/i when the engine rejects the steps.
func (c EngineStepCompiler) Compile(ctx context.Context, steps json.RawMessage, team uuid.UUID) error {
	var list config.Steps
	if err := json.Unmarshal(steps, &list); err != nil {
		return api.NewValidationError().Add(string(FieldSteps), string(CodeInvalid), err.Error())
	}
	raw, err := json.Marshal(c.document(list))
	if err != nil {
		return err
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return api.NewValidationError().Add(string(FieldSteps), string(CodeCompile), fmt.Sprintf(string(MsgCompile), err.Error()))
	}
	p, err := c.Engine.Compile(ctx, doc, c.Options.Options(team))
	if err != nil {
		return CompileIssues{}.Of(err)
	}
	return p.Close(ctx)
}

func (EngineStepCompiler) document(steps config.Steps) config.RouteConfig {
	ref := string(CheckRef)
	return config.RouteConfig{APIVersion: config.APIVersionRouterV1, Name: string(CheckName),
		Source: config.Source{ConnectionID: ref, Topics: []string{ref}, Format: config.Format{Type: config.FormatTypeJSON}},
		Branches: []config.Branch{{ID: string(CheckBranch), Transform: &config.Transform{Steps: steps},
			Destination: config.Destination{ConnectionID: ref, Topic: ref}}},
	}
}

// CompileIssues maps engine compile issues onto the publish request (/steps/i/...).
type CompileIssues struct{}

// Of returns a validation error for compile errors and err unchanged otherwise.
func (CompileIssues) Of(err error) error {
	var ce *engine.CompileError
	if !errors.As(err, &ce) {
		return api.NewValidationError().Add(string(FieldSteps), string(CodeCompile), fmt.Sprintf(string(MsgCompile), err.Error()))
	}
	verr := api.NewValidationError()
	for _, issue := range ce.Issues {
		path := strings.TrimPrefix(string(issue.Path), string(CheckStepsPrefix))
		if path == string(issue.Path) {
			path = string(FieldSteps)
		}
		verr.Add(path, string(CodeCompile), issue.Message)
	}
	if len(verr.Errors) == 0 {
		verr.Add(string(FieldSteps), string(CodeCompile), fmt.Sprintf(string(MsgCompile), err.Error()))
	}
	return verr
}
