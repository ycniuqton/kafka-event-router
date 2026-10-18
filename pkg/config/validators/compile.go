package validators

import (
	"context"
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// CompileValidator dry-runs engine.Compile and maps CompileError issues; the pipeline is closed.
type CompileValidator struct {
	Compiler engine.Compiler
	Options  engine.CompileOptions
}

func (CompileValidator) Name() config.ValidatorName { return config.ValidatorCompile }

func (v CompileValidator) Validate(ctx context.Context, vc *config.ValidationContext) []config.Issue {
	opts := v.Options
	opts.Meta.TeamID = vc.TeamID
	p, err := v.Compiler.Compile(ctx, vc.Doc, opts)
	if err == nil {
		_ = p.Close(ctx)
		return nil
	}
	var ce *engine.CompileError
	if errors.As(err, &ce) {
		return ce.Issues
	}
	return []config.Issue{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%v", err)}
}
