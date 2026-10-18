package filter

import "github.com/eventrouter/router/pkg/config"

// GroupCompiler compiles `and` / `or` nodes.
type GroupCompiler struct {
	NodeType config.NodeType
}

func (g GroupCompiler) Type() config.NodeType { return g.NodeType }

func (g GroupCompiler) Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool) {
	n := &groupNode{rel: rel, spec: raw, stopOn: g.NodeType == config.NodeTypeOr}
	ok := true
	for i, child := range raw.Children {
		cn, cok := c.Compile(rel.Token(config.TokenChildren).Index(i), child)
		ok = ok && cok
		n.children = append(n.children, cn)
	}
	return n, ok
}

// groupNode short-circuits: `and` stops on the first false, `or` on the first true.
type groupNode struct {
	rel      config.Pointer
	spec     config.FilterNode
	stopOn   bool
	children []Node
}

func (n *groupNode) Pointer() config.Pointer { return n.rel }

func (n *groupNode) Spec() config.FilterNode {
	s := n.spec
	s.Children = make([]config.FilterNode, 0, len(n.children))
	for _, c := range n.children {
		s.Children = append(s.Children, c.Spec())
	}
	return s
}

func (n *groupNode) Eval(ec *EvalContext) (bool, error) {
	result := !n.stopOn
	for _, c := range n.children {
		ok, err := c.Eval(ec)
		if err != nil {
			return false, err
		}
		if ok == n.stopOn {
			result = n.stopOn
			break
		}
	}
	ec.record(n, result, nil)
	return result, nil
}
