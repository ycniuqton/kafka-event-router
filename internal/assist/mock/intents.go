package mock

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// IntentRule recognises one kind of clause ("rename a to b", "mask email", …) and emits its steps.
type IntentRule interface {
	Pattern() *regexp.Regexp
	Build(groups []string, fields *FieldResolver) []config.Step
}

// MockIntentParser applies its rules in priority order; a text span is claimed by the first rule matching it.
type MockIntentParser struct {
	rules []IntentRule
}

// NewMockIntentParser registers the default clause rules (key before set, keep-only before remove).
func NewMockIntentParser() *MockIntentParser {
	return &MockIntentParser{rules: []IntentRule{
		renameRule{}, keyRule{}, keyAsRule{}, keepOnlyRule{}, defaultRule{}, setRule{}, maskRule{}, hashRule{}, removeRule{},
	}}
}

// Parse returns the steps of every recognised clause in the order they appear in the instruction.
func (p *MockIntentParser) Parse(instruction string, fields *FieldResolver) []config.Step {
	claims := &claimSet{text: instruction}
	for _, rule := range p.rules {
		for _, loc := range rule.Pattern().FindAllStringSubmatchIndex(instruction, -1) {
			claims.claim(rule, loc, fields)
		}
	}
	return claims.steps()
}

// MatchLayout describes regexp index slices: [start, end] of the match, then one pair per group.
type MatchLayout int

const (
	FirstGroupIndex MatchLayout = 2
	IndexPairWidth  MatchLayout = 2
)

// Submatches extracts the capture groups of one match.
type Submatches struct{}

// Of returns the text of every capture group (empty when a group did not participate).
func (Submatches) Of(s string, loc []int) []string {
	out := []string{}
	for i := int(FirstGroupIndex); i+1 < len(loc); i += int(IndexPairWidth) {
		if loc[i] < 0 {
			out = append(out, "")
			continue
		}
		out = append(out, s[loc[i]:loc[i+1]])
	}
	return out
}

// claim is one accepted match: its text span and its steps.
type claim struct {
	start, end int
	steps      []config.Step
}

// claimSet keeps non-overlapping matches; a match running into a later claim is cut before it and re-matched.
type claimSet struct {
	text   string
	claims []claim
}

func (c *claimSet) claim(rule IntentRule, loc []int, fields *FieldResolver) {
	if c.inside(loc[0]) {
		return
	}
	if end := c.nextStart(loc[0], loc[1]); end < loc[1] {
		loc = c.rematch(rule, loc[0], end)
	}
	if loc == nil {
		return
	}
	c.claims = append(c.claims, claim{start: loc[0], end: loc[1], steps: rule.Build(Submatches{}.Of(c.text, loc), fields)})
}

func (c *claimSet) inside(pos int) bool {
	for _, existing := range c.claims {
		if pos >= existing.start && pos < existing.end {
			return true
		}
	}
	return false
}

func (c *claimSet) nextStart(start, end int) int {
	for _, existing := range c.claims {
		if existing.start > start && existing.start < end {
			end = existing.start
		}
	}
	return end
}

// rematch runs rule on the cut text; the match must still start at the clause start.
func (c *claimSet) rematch(rule IntentRule, start, end int) []int {
	loc := rule.Pattern().FindStringSubmatchIndex(c.text[start:end])
	if loc == nil || loc[0] != 0 {
		return nil
	}
	for i := range loc {
		if loc[i] >= 0 {
			loc[i] += start
		}
	}
	return loc
}

func (c *claimSet) steps() []config.Step {
	sort.SliceStable(c.claims, func(i, j int) bool { return c.claims[i].start < c.claims[j].start })
	out := []config.Step{}
	for _, cl := range c.claims {
		out = append(out, cl.steps...)
	}
	return out
}

// ListSplitter splits "a, b and c" into names.
type ListSplitter struct{}

var listSeparator = regexp.MustCompile(`(?i)\s*(?:,|\band\b)\s*`)

// Split returns the non-empty items.
func (ListSplitter) Split(s string) []string {
	out := []string{}
	for _, item := range listSeparator.Split(s, -1) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Resolve splits s and resolves every name to a field path.
func (l ListSplitter) Resolve(s string, f *FieldResolver) []string {
	out := []string{}
	for _, name := range l.Split(s) {
		out = append(out, f.Resolve(name))
	}
	return out
}

// ValueLiteral turns an instruction value into JSON data: numbers, booleans, null, quoted or bare strings.
type ValueLiteral struct{}

// QuoteSingle is the alternative string quote accepted in instructions.
type QuoteSingle string

const SingleQuote QuoteSingle = "'"

// Parse decodes JSON literals and keeps anything else as a string.
func (ValueLiteral) Parse(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return strings.Trim(s, string(SingleQuote))
}
