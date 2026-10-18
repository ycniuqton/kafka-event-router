package mock

import (
	"fmt"
	"regexp"
	"strings"
)

// JsPattern is one recognisable operation in user JavaScript and the bullet describing it.
type JsPattern struct {
	Match  *regexp.Regexp
	Bullet string
}

// ExplainText holds the summary templates of the mock explainer.
type ExplainText string

const (
	TextSummary      ExplainText = "Rewrites each event with custom JavaScript (%d notable operations)."
	TextPlainSummary ExplainText = "Returns each event after custom JavaScript logic."
	TextEmpty        ExplainText = "This JavaScript step has no code yet."
	TextBullet       ExplainText = "- "
	TextNewline      ExplainText = "\n"
)

// DefaultJsPatterns are checked in order; each matching pattern adds one bullet.
var DefaultJsPatterns = []JsPattern{
	{regexp.MustCompile(`\.\.\.\s*[\w.]+`), "Keeps every existing field and overrides some (object spread)."},
	{regexp.MustCompile(`\.reduce\(`), "Aggregates an array into one value (reduce)."},
	{regexp.MustCompile(`\.map\(`), "Transforms each element of an array (map)."},
	{regexp.MustCompile(`\.filter\(`), "Keeps only some array elements (filter)."},
	{regexp.MustCompile(`ctx\.now\(`), "Adds the processing time (ctx.now)."},
	{regexp.MustCompile(`ctx\.mask\.`), "Masks sensitive values (ctx.mask)."},
	{regexp.MustCompile(`ctx\.hash\(`), "Hashes values with the team salt (ctx.hash)."},
	{regexp.MustCompile(`ctx\.lookup\(`), "Looks values up in a table (ctx.lookup)."},
	{regexp.MustCompile(`ctx\.formatDate\(`), "Formats dates (ctx.formatDate)."},
	{regexp.MustCompile(`ctx\.log\(`), "Writes lines to the console (ctx.log)."},
	{regexp.MustCompile(`\bdelete\s+`), "Deletes fields."},
	{regexp.MustCompile(`return\s+null\b`), "Drops some events (returns null)."},
	{regexp.MustCompile(`return\s*\[`), "Can emit several events (returns an array)."},
	{regexp.MustCompile(`\bthrow\b`), "Fails on purpose for some events (throw); they go to the dead-letter queue."},
}

// MockExplainer summarises JavaScript by the operations it recognises.
type MockExplainer struct {
	patterns []JsPattern
}

// NewMockExplainer uses DefaultJsPatterns.
func NewMockExplainer() *MockExplainer {
	return &MockExplainer{patterns: DefaultJsPatterns}
}

// Explain returns the completion text: a summary line followed by "- " bullet lines.
func (e *MockExplainer) Explain(code string) string {
	if strings.TrimSpace(code) == "" {
		return string(TextEmpty)
	}
	bullets := []string{}
	for _, p := range e.patterns {
		if p.Match.MatchString(code) {
			bullets = append(bullets, string(TextBullet)+p.Bullet)
		}
	}
	summary := string(TextPlainSummary)
	if len(bullets) > 0 {
		summary = fmt.Sprintf(string(TextSummary), len(bullets))
	}
	return strings.Join(append([]string{summary}, bullets...), string(TextNewline))
}
