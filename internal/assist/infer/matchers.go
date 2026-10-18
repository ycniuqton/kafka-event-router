package infer

import (
	"fmt"
	"strings"
	"unicode"
)

// LeafMatcher is the base of every matching strategy: it proposes a source leaf for one target leaf.
// Register implementations in a MatcherManager; the configured order decides who wins.
type LeafMatcher interface {
	Kind() Reason
	Match(target Leaf, pool *SourcePool) (Match, bool)
}

// SourcePool holds the input leaves, how often each was used as a source, and the paths the output
// overwrites (reserved leaves are no free guesses for the type matcher).
type SourcePool struct {
	Leaves   []Leaf
	uses     map[int]int
	reserved map[string]bool
}

// NewSourcePool wraps the input leaves; targets are the desired output's leaves.
func NewSourcePool(leaves, targets []Leaf) *SourcePool {
	p := &SourcePool{Leaves: leaves, uses: map[int]int{}, reserved: map[string]bool{}}
	for _, t := range targets {
		p.reserved[t.Path()] = true
	}
	return p
}

// Free reports whether leaf i is neither used nor overwritten by a target.
func (p *SourcePool) Free(i int) bool { return !p.Used(i) && !p.reserved[p.Leaves[i].Path()] }

// Use records that leaf i feeds a target.
func (p *SourcePool) Use(i int) { p.uses[i]++ }

// Used reports whether leaf i feeds any target.
func (p *SourcePool) Used(i int) bool { return p.uses[i] > 0 }

// Release forgets one use of leaf i (a match turned into a constant).
func (p *SourcePool) Release(i int) { p.uses[i]-- }

// Uses returns how many targets leaf i feeds.
func (p *SourcePool) Uses(i int) int { return p.uses[i] }

// Index returns the position of leaf l in the pool (-1 when absent).
func (p *SourcePool) Index(l *Leaf) int {
	for i := range p.Leaves {
		if &p.Leaves[i] == l {
			return i
		}
	}
	return -1
}

// pick returns the best of candidate indexes: unused first, then the one whose name matches target.
func (p *SourcePool) pick(candidates []int, target Leaf) int {
	best, bestScore := -1, -1
	for _, i := range candidates {
		score := 0
		if !p.Used(i) {
			score += 2
		}
		if (Names{}).Key(p.Leaves[i].Name()) == (Names{}).Key(target.Name()) {
			score++
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

// EqualValueMatcher pairs leaves holding the same value (1.0; 0.8 when several inputs hold it).
type EqualValueMatcher struct{}

func (EqualValueMatcher) Kind() Reason { return ReasonValue }

// Match implements LeafMatcher.
func (EqualValueMatcher) Match(target Leaf, pool *SourcePool) (Match, bool) {
	key := (Values{}).Key(target.Value)
	candidates := []int{}
	for i, l := range pool.Leaves {
		if (Values{}).Key(l.Value) == key {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return Match{}, false
	}
	conf := ConfidenceExact
	if len(candidates) > 1 {
		conf = ConfidenceAmbiguous
	}
	return Match{Target: target, Source: &pool.Leaves[pool.pick(candidates, target)], Reason: ReasonValue, Confidence: conf}, true
}

// NormalisedNameMatcher pairs leaves whose names match ignoring case, '_' and '-' (0.7).
type NormalisedNameMatcher struct{}

func (NormalisedNameMatcher) Kind() Reason { return ReasonName }

// Match implements LeafMatcher.
func (NormalisedNameMatcher) Match(target Leaf, pool *SourcePool) (Match, bool) {
	name := (Names{}).Key(target.Name())
	candidates := []int{}
	for i, l := range pool.Leaves {
		if (Names{}).Key(l.Name()) == name {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return Match{}, false
	}
	return Match{Target: target, Source: &pool.Leaves[pool.pick(candidates, target)], Reason: ReasonName, Confidence: ConfidenceName}, true
}

// TypeShapeMatcher pairs a target with the first free input leaf of the same kind and depth (0.4).
type TypeShapeMatcher struct{}

func (TypeShapeMatcher) Kind() Reason { return ReasonType }

// Match implements LeafMatcher.
func (TypeShapeMatcher) Match(target Leaf, pool *SourcePool) (Match, bool) {
	for i, l := range pool.Leaves {
		if pool.Free(i) && l.Kind == target.Kind && len(l.Segments) == len(target.Segments) {
			return Match{Target: target, Source: &pool.Leaves[i], Reason: ReasonType, Confidence: ConfidenceType}, true
		}
	}
	return Match{}, false
}

// Names normalises field names for comparison.
type Names struct{}

// Key returns the name in lower case without '_', '-' and spaces.
func (n Names) Key(name string) string {
	return strings.Map(n.rune, name)
}

// rune maps one character of a name (-1 drops it).
func (Names) rune(r rune) rune {
	if r == '_' || r == '-' || unicode.IsSpace(r) {
		return -1
	}
	return unicode.ToLower(r)
}

// MatcherManager runs the configured matcher order; the first match per target leaf wins.
type MatcherManager struct {
	chain []LeafMatcher
}

// DefaultOrder is value, name, type (design §3.2).
var DefaultOrder = []Reason{ReasonValue, ReasonName, ReasonType}

// NewMatcherManager orders the registered matchers by order; an unknown kind is an error.
func NewMatcherManager(order []Reason, matchers ...LeafMatcher) (*MatcherManager, error) {
	byKind := map[Reason]LeafMatcher{}
	for _, m := range matchers {
		byKind[m.Kind()] = m
	}
	m := &MatcherManager{}
	for _, kind := range order {
		lm, ok := byKind[kind]
		if !ok {
			return nil, fmt.Errorf("infer: unknown matcher %q", kind)
		}
		m.chain = append(m.chain, lm)
	}
	return m, nil
}

// DefaultMatcherManager registers every shipped matcher in order (DefaultOrder when order is empty).
func DefaultMatcherManager(order []Reason) (*MatcherManager, error) {
	if len(order) == 0 {
		order = DefaultOrder
	}
	return NewMatcherManager(order, EqualValueMatcher{}, NormalisedNameMatcher{}, TypeShapeMatcher{})
}

// Match returns the first match of the chain and records the source use.
func (m *MatcherManager) Match(target Leaf, pool *SourcePool) (Match, bool) {
	for _, lm := range m.chain {
		if match, ok := lm.Match(target, pool); ok {
			pool.Use(pool.Index(match.Source))
			return match, true
		}
	}
	return Match{}, false
}
