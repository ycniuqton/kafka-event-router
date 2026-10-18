package diff

import (
	"github.com/eventrouter/router/internal/routes/summary"
	"github.com/eventrouter/router/pkg/config"
)

// FilterSection lists every filter node as a readable row keyed by its position.
type FilterSection struct{}

func (FilterSection) Section() SectionKind { return SectionFilter }

func (FilterSection) Entries(cfg *config.RouteConfig) []Entry {
	es := Entries{}
	for i, b := range cfg.Branches {
		if b.Filter == nil {
			continue
		}
		w := filterRows{prefix: BranchLabel{}.For(cfg, b), entries: &es}
		w.visit(b.Filter, config.BranchPointer(i).Token(config.TokenFilter))
	}
	return es
}

// groupLabels names the header line of each group node type.
var groupLabels = map[config.NodeType]Label{
	config.NodeTypeAnd: LabelMatchAll,
	config.NodeTypeOr:  LabelMatchAny,
	config.NodeTypeNot: LabelNot,
}

// filterRows walks a filter tree into entries.
type filterRows struct {
	prefix  string
	entries *Entries
}

func (w filterRows) visit(n *config.FilterNode, at config.Pointer) {
	label, isGroup := groupLabels[n.Type]
	if !isGroup {
		w.entries.AddPrefixed(string(at), at, w.prefix, summary.FilterSummarizer{}.Text(n))
		return
	}
	w.entries.AddPrefixed(string(at)+string(KeySep)+string(KeyFilterTag), at.Token(config.TokenType), w.prefix, string(label))
	for i := range n.Children {
		w.visit(&n.Children[i], at.Token(config.TokenChildren).Index(i))
	}
	if n.Child != nil {
		w.visit(n.Child, at.Token(config.TokenChild))
	}
}
