package formula

import "github.com/eventrouter/router/pkg/engine/path"

// Fields lists the event fields the formula reads; ok is false when the AST holds a node
// whose reads are unknown, so callers must assume the whole event is read.
func (p *Program) Fields() ([]path.FieldPath, bool) {
	var out []path.FieldPath
	ok := FieldCollector{}.collect(p.root, &out)
	return out, ok
}

// FieldCollector walks compiled formula nodes for field references.
type FieldCollector struct{}

func (c FieldCollector) collect(e Expr, out *[]path.FieldPath) bool {
	switch n := e.(type) {
	case literal:
		return true
	case fieldRef:
		*out = append(*out, n.path)
		return true
	case unary:
		return c.collect(n.operand, out)
	case binary:
		return c.collect(n.left, out) && c.collect(n.right, out)
	case call:
		return c.all(n.args, out)
	}
	return false
}

func (c FieldCollector) all(args []Expr, out *[]path.FieldPath) bool {
	for _, a := range args {
		if !c.collect(a, out) {
			return false
		}
	}
	return true
}
