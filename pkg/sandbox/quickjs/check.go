package quickjs

import (
	"context"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Check compiles src and, for transforms, loads it once to verify transform() exists
// and that top-level code runs; problems become error diagnostics at src.StepPath.
func (r *Runtime) Check(ctx context.Context, src sandbox.Source) []sandbox.Diagnostic {
	prog, err := r.Compile(ctx, src, r.opts.CheckLimits)
	if err != nil {
		return r.diagnose(src, err)
	}
	if src.Entry != sandbox.EntryTransform {
		return nil
	}
	inst, err := prog.NewInstance(ctx, host.NewStandardRegistry(nil))
	if err != nil {
		return r.diagnose(src, err)
	}
	_ = inst.Close(ctx)
	return nil
}

func (r *Runtime) diagnose(src sandbox.Source, err error) []sandbox.Diagnostic {
	return []sandbox.Diagnostic{sandbox.DiagnosticFromError(src.StepPath, r.classifier.Classify(err))}
}
