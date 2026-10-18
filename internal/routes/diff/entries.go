package diff

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// Entry is one comparable fact of a config: entries with the same key are compared by text.
type Entry struct {
	Key    string
	Path   config.Pointer
	Prefix string
	Text   string
}

// Display is the shown line: the optional branch prefix plus the compared text.
func (e Entry) Display() string {
	return e.Prefix + e.Text
}

// Entries is an ordered entry list being built.
type Entries []Entry

// Add appends an entry; empty texts are skipped.
func (es *Entries) Add(key string, path config.Pointer, text string) {
	es.AddPrefixed(key, path, "", text)
}

// AddPrefixed appends an entry whose prefix (branch label) is shown but not compared.
func (es *Entries) AddPrefixed(key string, path config.Pointer, prefix, text string) {
	if text == "" {
		return
	}
	*es = append(*es, Entry{Key: key, Path: path, Prefix: prefix, Text: text})
}

// EntryComparer turns two entry lists into diff lines: a's order first, then b-only entries.
type EntryComparer struct{}

// ChangeArrow separates the old and new text of a changed line.
type ChangeArrow string

const changeArrow ChangeArrow = " → "

// Compare returns removed / changed lines in a's order followed by added lines in b's order.
func (EntryComparer) Compare(a, b []Entry) []Line {
	inB := map[string]Entry{}
	for _, e := range b {
		inB[e.Key] = e
	}
	inA := map[string]bool{}
	lines := []Line{}
	for _, e := range a {
		inA[e.Key] = true
		other, ok := inB[e.Key]
		switch {
		case !ok:
			lines = append(lines, Line{Mark: MarkRemoved, Text: e.Display(), Path: string(e.Path)})
		case other.Text != e.Text:
			lines = append(lines, Line{Mark: MarkChanged, Text: other.Prefix + e.Text + string(changeArrow) + other.Text, Path: string(other.Path)})
		}
	}
	for _, e := range b {
		if !inA[e.Key] {
			lines = append(lines, Line{Mark: MarkAdded, Text: e.Display(), Path: string(e.Path)})
		}
	}
	return lines
}

// TextLimit bounds rendered values in diff lines.
type TextLimit int

const maxValueText TextLimit = 120

// Text renders optional values for diff lines.
type Text struct{}

// Str dereferences an optional string.
func (Text) Str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Int renders an optional int.
func (Text) Int(i *int) string {
	if i == nil {
		return ""
	}
	return fmt.Sprint(*i)
}

// Labeled prefixes a non-empty value with its label.
func (Text) Labeled(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + " " + value
}

// JSON renders any value compactly, truncated; empty values render as "".
func (t Text) JSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil || string(raw) == "null" || string(raw) == "{}" {
		return ""
	}
	return t.Truncate(string(raw))
}

// Truncate shortens long values.
func (Text) Truncate(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= int(maxValueText) {
		return string(r)
	}
	return string(r[:maxValueText]) + "…"
}

// BranchLabel prefixes per-branch lines when a route has several branches.
type BranchLabel struct{}

// For returns "[id] " for multi-branch routes, else "".
func (BranchLabel) For(cfg *config.RouteConfig, b config.Branch) string {
	if len(cfg.Branches) <= 1 {
		return ""
	}
	return "[" + b.ID + "] "
}
