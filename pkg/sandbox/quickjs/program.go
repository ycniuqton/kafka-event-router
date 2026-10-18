package quickjs

import (
	"context"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// program is user code validated once per route version (syntax checked by the compiler guest).
type program struct {
	rt     *Runtime
	src    sandbox.Source
	guest  guestSource
	limits sandbox.Limits
}

func (p *program) Source() sandbox.Source { return p.src }

func (p *program) Limits() sandbox.Limits { return p.limits }

// Compile wraps src and syntax-checks it once (errors carry line/column).
func (r *Runtime) Compile(ctx context.Context, src sandbox.Source, lim sandbox.Limits) (sandbox.Program, error) {
	gs, serr := newGuestSource(src)
	if serr != nil {
		return nil, serr
	}
	if err := r.compiler.compile(ctx, gs); err != nil {
		return nil, err
	}
	return &program{rt: r, src: src, guest: gs, limits: lim}, nil
}

// NewInstance boots a guest, loads the program and seals it per the reset policy.
func (p *program) NewInstance(ctx context.Context, registry *host.Registry) (sandbox.Instance, error) {
	return instanceLoader{rt: p.rt, program: p, registry: registry}.load(ctx)
}

// instanceLoader runs the load sequence of one instance.
type instanceLoader struct {
	rt       *Runtime
	program  *program
	registry *host.Registry
}

// load instantiates the module, boots QuickJS, runs the program and captures the snapshot.
func (l instanceLoader) load(ctx context.Context) (*instance, error) {
	lim := l.program.limits
	grp, err := l.rt.group(ctx, lim.MemoryBytes)
	if err != nil {
		return nil, err
	}
	loadCtx, cancel := context.WithTimeout(ctx, instanceLoadTimeout.duration())
	defer cancel()
	mod, err := grp.newModule(loadCtx)
	if err != nil {
		return nil, l.classify(loadCtx, err)
	}
	inst, err := l.boot(loadCtx, grp, mod)
	if err != nil {
		_ = mod.Close(ctx)
		return nil, l.classify(loadCtx, err)
	}
	return inst, nil
}

// boot clones the group's booted template into mod and loads the program.
func (l instanceLoader) boot(ctx context.Context, grp *engineGroup, mod moduleHandle) (*instance, error) {
	lim := l.program.limits
	tpl, err := grp.template(ctx, l.rt.opts.Prelude, budgetFor(lim.MemoryBytes).heapLimit)
	if err != nil {
		return nil, err
	}
	vm, err := tpl.clone(mod)
	if err != nil {
		return nil, err
	}
	states := stateFactory{abi: vm.abi, registry: l.registry, limits: lim}
	ctx = withCallState(ctx, states.forLoad())
	if err = vm.loadProgram(ctx, l.program.guest); err != nil {
		return nil, err
	}
	inst := &instance{mod: mod, vm: vm, states: states, limits: lim, reusable: true,
		traps:   trapClassifier{classifier: l.rt.classifier, limits: lim},
		decoder: replyDecoder{classifier: l.rt.classifier, limits: lim, position: vm.position}}
	return inst, l.seal(ctx, inst)
}

// seal applies the reset policy: snapshot for fresh_context, frozen intrinsics for lockdown_reuse.
func (l instanceLoader) seal(ctx context.Context, inst *instance) error {
	if l.rt.opts.ResetPolicy == sandbox.ResetLockdownReuse {
		return inst.vm.lockdown(ctx)
	}
	snap, err := captureSnapshot(ctx, inst.vm.abi, budgetFor(l.program.limits.MemoryBytes).headroom)
	inst.snapshot = snap
	return err
}

func (l instanceLoader) classify(ctx context.Context, err error) error {
	if _, ok := sandbox.AsError(err); ok {
		return err
	}
	return trapClassifier{classifier: l.rt.classifier, limits: l.program.limits}.classify(ctx, err)
}
