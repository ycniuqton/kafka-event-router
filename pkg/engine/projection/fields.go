package projection

import (
	"github.com/eventrouter/router/pkg/engine/path"
)

// FieldNode is one object member of the projection. A full node is decoded with its whole
// subtree; a partial node is an object of which only Children are decoded.
type FieldNode struct {
	Name     string
	Children map[string]*FieldNode
	Full     bool
}

// Child returns the child named name, or nil.
func (n *FieldNode) Child(name string) *FieldNode {
	if n == nil || n.Full {
		return nil
	}
	return n.Children[name]
}

// Partial reports whether n is decoded member by member.
func (n *FieldNode) Partial() bool { return n != nil && !n.Full }

// FieldTree is the set of value members a route can read or write, rooted at the value.
type FieldTree struct {
	Root *FieldNode
}

// FieldTreeBuilder merges value paths into a tree: a path ends in a full node, and an
// array index makes the member before it full (arrays are decoded whole).
type FieldTreeBuilder struct{}

// Build returns the tree of the value paths in ps (other roots are ignored).
func (FieldTreeBuilder) Build(ps []path.FieldPath) FieldTree {
	root := &FieldNode{Children: map[string]*FieldNode{}}
	for _, p := range ps {
		if p.Root == path.RootValue && !p.IsRoot() {
			FieldTreeBuilder{}.add(root, p.Segments)
		}
	}
	return FieldTree{Root: root}
}

func (FieldTreeBuilder) add(n *FieldNode, segs []path.Segment) {
	for i, s := range segs {
		if n.Full {
			return
		}
		child := n.Children[s.Name]
		if child == nil {
			child = &FieldNode{Name: s.Name, Children: map[string]*FieldNode{}}
			n.Children[s.Name] = child
		}
		if i == len(segs)-1 || segs[i+1].IsIndex {
			child.Full, child.Children = true, nil
			return
		}
		n = child
	}
}
