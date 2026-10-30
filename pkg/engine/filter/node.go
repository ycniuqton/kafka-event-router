// Package filter compiles the filter JSON AST (and, or, not, cond, expr) into an
// evaluable node tree with short-circuit evaluation and per-node tracing.
package filter

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// EvalContext is the per-event state of one filter evaluation.
type EvalContext struct {
	Ctx      context.Context
	Event    *engine.Event
	Now      time.Time
	Branch   engine.BranchID
	Tracer   trace.Tracer
	Coercer  Coercer
	snapshot map[string]any
}

// Snapshot returns the JSON-shaped event, built once per evaluation.
func (ec *EvalContext) Snapshot() map[string]any {
	if ec.snapshot == nil {
		ec.snapshot = ec.Event.Snapshot()
	}
	return ec.snapshot
}

// record forwards a node result to the tracer.
func (ec *EvalContext) record(n Node, ok bool, w *CoercionWarning) {
	if !ec.Tracer.Enabled() {
		return
	}
	ec.Tracer.Node(string(ec.Branch), string(n.Pointer()), ok, w.CodeString())
}

// Node is one compiled filter node; Pointer is relative to the filter root.
type Node interface {
	Pointer() config.Pointer
	Spec() config.FilterNode
	Eval(ec *EvalContext) (bool, error)
}

// NodeCompileCtx is the compile state of one filter tree.
type NodeCompileCtx struct {
	Scope     *compilectx.Scope
	Base      config.Pointer
	Nodes     *NodeCompilerManager
	Operators *OperatorManager
}

// Abs returns the document pointer of a node pointer.
func (c *NodeCompileCtx) Abs(rel config.Pointer) config.Pointer { return c.Base.Join(rel) }

// Compile compiles a (child) node at rel.
func (c *NodeCompileCtx) Compile(rel config.Pointer, raw config.FilterNode) (Node, bool) {
	return c.Nodes.Compile(c, rel, raw)
}

// NodeCompiler compiles one node type.
type NodeCompiler interface {
	Type() config.NodeType
	Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool)
}

// NodeCompilerManager selects node compilers by node type.
type NodeCompilerManager struct {
	compilers map[config.NodeType]NodeCompiler
}

// NewNodeCompilerManager registers the given compilers.
func NewNodeCompilerManager(cs ...NodeCompiler) *NodeCompilerManager {
	m := &NodeCompilerManager{compilers: map[config.NodeType]NodeCompiler{}}
	for _, c := range cs {
		m.compilers[c.Type()] = c
	}
	return m
}

// DefaultNodeCompilerManager registers and, or, not, cond and expr.
func DefaultNodeCompilerManager() *NodeCompilerManager {
	return NewNodeCompilerManager(GroupCompiler{NodeType: config.NodeTypeAnd}, GroupCompiler{NodeType: config.NodeTypeOr},
		NotCompiler{}, CondCompiler{}, ExprCompiler{})
}

// Compile dispatches by raw.Type; unknown types become SchemaInvalid issues.
func (m *NodeCompilerManager) Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool) {
	nc, ok := m.compilers[raw.Type]
	if !ok {
		c.Scope.Fail(config.IssueSchemaInvalid, c.Abs(rel).Token(config.TokenType), "unknown filter node type %q", raw.Type)
		return nil, false
	}
	c.Scope.Access.Node(raw.Type)
	return nc.Compile(c, rel, raw)
}

// FilterError wraps a node evaluation failure with its pointer.
type FilterError struct {
	Pointer config.Pointer
	Cause   error
}

func (e *FilterError) Error() string { return fmt.Sprintf("filter node %q: %v", e.Pointer, e.Cause) }

func (e *FilterError) Unwrap() error { return e.Cause }
