package step

import "github.com/eventrouter/router/pkg/engine/path"

// FieldAccess lists the event fields a compiled step reads and writes; FullTree means the step
// may touch anything (codecs must then decode and re-encode the whole event).
type FieldAccess struct {
	Reads    []path.FieldPath
	Writes   []path.FieldPath
	FullTree bool
}

// FieldAccessor is implemented by steps that declare their field access (fast-codec field analyser);
// steps without it are treated as FullTree.
type FieldAccessor interface {
	FieldAccess() FieldAccess
}

// FieldAccessOf returns the declared access of s, or FullTree when s declares none.
type FieldAccessOf struct{}

// Of returns the access of one compiled step.
func (FieldAccessOf) Of(s Step) FieldAccess {
	if a, ok := s.(FieldAccessor); ok {
		return a.FieldAccess()
	}
	return FieldAccess{FullTree: true}
}
