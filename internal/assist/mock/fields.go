package mock

import (
	"sort"
	"strings"
	"unicode"
)

// PathRoot holds the event roots a field path may start with.
type PathRoot string

const (
	RootValue   PathRoot = "value."
	RootHeaders PathRoot = "headers."
	RootKey     PathRoot = "key"
)

// PathSeparator separates path segments.
type PathSeparator string

const Dot PathSeparator = "."

// FieldResolver maps the words of an instruction onto the sample's field paths (fuzzy, deterministic).
type FieldResolver struct {
	fields []string
}

// NewFieldResolver keeps the known paths, shortest first so the least nested match wins.
func NewFieldResolver(fields []string) *FieldResolver {
	sorted := append([]string{}, fields...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) < len(sorted[j]) })
	return &FieldResolver{fields: sorted}
}

// Resolve returns the full path of word: rooted words as is, else exact, normalised, then last-segment match.
func (r *FieldResolver) Resolve(word string) string {
	if r.rooted(word) {
		return word
	}
	matchers := []func(rel, field string) bool{
		func(rel, _ string) bool { return rel == word },
		func(rel, _ string) bool { return Normaliser{}.Of(rel) == Normaliser{}.Of(word) },
		func(rel, _ string) bool { return Normaliser{}.Of(r.last(rel)) == Normaliser{}.Of(word) },
	}
	for _, match := range matchers {
		if found, ok := r.first(match); ok {
			return found
		}
	}
	return string(RootValue) + word
}

// Sibling places name next to the resolved source path (rename targets); dotted names start at value.
func (r *FieldResolver) Sibling(source, name string) string {
	if strings.Contains(name, string(Dot)) || r.rooted(name) {
		return r.rootedOrValue(name)
	}
	idx := strings.LastIndex(source, string(Dot))
	if idx < 0 {
		return string(RootValue) + name
	}
	return source[:idx+1] + name
}

func (r *FieldResolver) first(match func(rel, field string) bool) (string, bool) {
	for _, f := range r.fields {
		if match(strings.TrimPrefix(f, string(RootValue)), f) {
			return f, true
		}
	}
	return "", false
}

func (r *FieldResolver) rooted(word string) bool {
	return strings.HasPrefix(word, string(RootValue)) || strings.HasPrefix(word, string(RootHeaders)) || word == string(RootKey)
}

func (r *FieldResolver) rootedOrValue(word string) string {
	if r.rooted(word) {
		return word
	}
	return string(RootValue) + word
}

func (r *FieldResolver) last(rel string) string {
	return rel[strings.LastIndex(rel, string(Dot))+1:]
}

// Normaliser compares names ignoring case and separators (cust_id ≈ custId ≈ CUST-ID).
type Normaliser struct{}

// Of keeps only lower-cased letters and digits.
func (Normaliser) Of(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
