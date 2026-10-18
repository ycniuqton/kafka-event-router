// Package summary renders route configs as short readable text: the filter headline of the
// routes list, condition rows of the diff and the transform indicator.
package summary

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// TextToken names the connective words of rendered filters.
type TextToken string

const (
	TokenAnd        TextToken = " and "
	TokenOr         TextToken = " or "
	TokenNot        TextToken = "not "
	TokenExpr       TextToken = "expression: "
	TokenListSep    TextToken = ", "
	TokenNull       TextToken = "null"
	TokenEllipsis   TextToken = "…"
	TokenBetweenAnd TextToken = " and "
	TokenOpen       TextToken = "("
	TokenClose      TextToken = ")"
	EventValuePath  TextToken = "value."
)

// ExprPreviewLimit bounds how many characters of an expression are shown.
type ExprPreviewLimit int

const exprPreviewLimit ExprPreviewLimit = 60

// OperatorPhrase maps engine operators to the words shown to users.
var OperatorPhrase = map[config.OpName]string{
	config.OpNameEq:         "is",
	config.OpNameNe:         "is not",
	config.OpNameGt:         ">",
	config.OpNameGte:        "≥",
	config.OpNameLt:         "<",
	config.OpNameLte:        "≤",
	config.OpNameContains:   "contains",
	config.OpNameStartsWith: "starts with",
	config.OpNameEndsWith:   "ends with",
	config.OpNameIn:         "is one of",
	config.OpNameNotIn:      "is not one of",
	config.OpNameExists:     "exists",
	config.OpNameNotExists:  "does not exist",
	config.OpNameIsEmpty:    "is empty",
	config.OpNameRegex:      "matches",
	config.OpNameBetween:    "is between",
	config.OpNameBefore:     "is before",
	config.OpNameAfter:      "is after",
	config.OpNameWithinLast: "is within the last",
}

// FilterHeadline is the list column: the first condition plus how many more there are.
type FilterHeadline struct {
	Headline string `json:"headline"`
	More     int    `json:"more"`
}

// FilterSummarizer renders filter ASTs as text.
type FilterSummarizer struct{}

// Headline summarises a filter for the list; nil means the route keeps every event.
func (s FilterSummarizer) Headline(node *config.FilterNode) *FilterHeadline {
	if node == nil {
		return nil
	}
	if s.isGroup(node.Type) && len(node.Children) > 0 {
		return &FilterHeadline{Headline: s.Text(&node.Children[0]), More: len(node.Children) - 1}
	}
	return &FilterHeadline{Headline: s.Text(node)}
}

// Text renders any node, groups in parentheses when nested.
func (s FilterSummarizer) Text(node *config.FilterNode) string {
	switch node.Type {
	case config.NodeTypeAnd, config.NodeTypeOr:
		return s.group(node)
	case config.NodeTypeNot:
		return s.negation(node)
	case config.NodeTypeExpr:
		return string(TokenExpr) + s.truncate(s.deref(node.Code))
	default:
		return CondText{}.Render(node)
	}
}

func (s FilterSummarizer) group(node *config.FilterNode) string {
	sep := TokenAnd
	if node.Type == config.NodeTypeOr {
		sep = TokenOr
	}
	parts := make([]string, 0, len(node.Children))
	for i := range node.Children {
		parts = append(parts, s.nested(&node.Children[i]))
	}
	return strings.Join(parts, string(sep))
}

func (s FilterSummarizer) negation(node *config.FilterNode) string {
	if node.Child == nil {
		return string(TokenNot)
	}
	return string(TokenNot) + s.nested(node.Child)
}

func (s FilterSummarizer) nested(node *config.FilterNode) string {
	if s.isGroup(node.Type) {
		return string(TokenOpen) + s.Text(node) + string(TokenClose)
	}
	return s.Text(node)
}

func (FilterSummarizer) isGroup(t config.NodeType) bool {
	return t == config.NodeTypeAnd || t == config.NodeTypeOr
}

func (FilterSummarizer) deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (FilterSummarizer) truncate(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= int(exprPreviewLimit) {
		return string(r)
	}
	return string(r[:exprPreviewLimit]) + string(TokenEllipsis)
}

// CondText renders one `cond` node as "field op value".
type CondText struct{}

// Render returns e.g. "amount > 100" or "eventType is one of A, B".
func (c CondText) Render(node *config.FilterNode) string {
	field := c.field(node.Field)
	if node.Op == nil {
		return field
	}
	phrase, ok := OperatorPhrase[*node.Op]
	if !ok {
		phrase = string(*node.Op)
	}
	operand := c.operand(node)
	if operand == "" {
		return field + " " + phrase
	}
	return field + " " + phrase + " " + operand
}

func (c CondText) operand(node *config.FilterNode) string {
	switch *node.Op {
	case config.OpNameExists, config.OpNameNotExists, config.OpNameIsEmpty:
		return ""
	case config.OpNameIn, config.OpNameNotIn:
		return c.list(node.Values)
	case config.OpNameBetween:
		return c.value(node.From) + string(TokenBetweenAnd) + c.value(node.To)
	case config.OpNameWithinLast:
		return c.duration(node)
	default:
		return c.value(node.Value)
	}
}

func (c CondText) duration(node *config.FilterNode) string {
	amount := ""
	if node.Amount != nil {
		amount = fmt.Sprint(*node.Amount)
	}
	if node.Unit == nil {
		return amount
	}
	return amount + " " + string(*node.Unit)
}

func (CondText) field(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimPrefix(*p, string(EventValuePath))
}

func (c CondText) list(vs []any) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, c.value(v))
	}
	return strings.Join(parts, string(TokenListSep))
}

func (CondText) value(v any) string {
	if v == nil {
		return string(TokenNull)
	}
	return fmt.Sprint(v)
}
