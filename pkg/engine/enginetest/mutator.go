package enginetest

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"unicode"
	"unicode/utf16"

	"github.com/eventrouter/router/pkg/engine/codec"
)

// MutationRate holds the odds (one in N) of each random change the mutator makes.
type MutationRate int

const (
	rateCorrupt   MutationRate = 12
	rateReplace   MutationRate = 5
	rateDelete    MutationRate = 9
	rateAdd       MutationRate = 6
	rateSpace     MutationRate = 4
	rateDuplicate MutationRate = 7
	rateEscape    MutationRate = 5
	rateNumber    MutationRate = 3
	maxDepth      MutationRate = 4
)

// EventMutator derives random variants of JSON events for differential tests: changed,
// removed and added members (any type, nested), shuffled keys, whitespace, duplicate
// keys, escapes, number spellings, unicode, and some invalid documents.
type EventMutator struct {
	R    *rand.Rand
	Keys []string
}

// Variant returns one random variant of base.
func (m EventMutator) Variant(base []byte) []byte {
	if m.odds(rateCorrupt) {
		return m.corrupt(base)
	}
	tree, err := codec.TreeReader{}.Read(base)
	if err != nil {
		return base
	}
	var b bytes.Buffer
	m.write(&b, m.mutate(tree, 0), 0)
	return b.Bytes()
}

func (m EventMutator) odds(r MutationRate) bool { return m.R.IntN(int(r)) == 0 }

func (m EventMutator) corrupt(base []byte) []byte {
	out := append([]byte(nil), base...)
	if len(out) == 0 {
		return []byte("{")
	}
	switch m.R.IntN(4) {
	case 0:
		return out[:m.R.IntN(len(out))]
	case 1:
		out[m.R.IntN(len(out))] = byte(m.R.IntN(256))
		return out
	case 2:
		return append(out, []byte(" x")...)
	}
	return append([]byte("  \n"), append(out, '\t')...)
}

func (m EventMutator) mutate(v any, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = m.member(e, depth)
			if m.odds(rateDelete) {
				delete(t, k)
			}
		}
		if m.odds(rateAdd) {
			t[m.key()] = m.random(depth + 1)
		}
	case []any:
		for i, e := range t {
			t[i] = m.member(e, depth)
		}
	}
	return v
}

func (m EventMutator) member(v any, depth int) any {
	if m.odds(rateReplace) {
		return m.random(depth + 1)
	}
	return m.mutate(v, depth+1)
}

func (m EventMutator) key() string {
	pool := append([]string{"a", "b", "x", "items", "id", "é", "k\"q", " "}, m.Keys...)
	return pool[m.R.IntN(len(pool))]
}

// random returns a random value of any JSON type.
func (m EventMutator) random(depth int) any {
	n := 9
	if depth >= int(maxDepth) {
		n = 6
	}
	switch m.R.IntN(n) {
	case 0:
		return nil
	case 1:
		return m.R.IntN(2) == 0
	case 2:
		return m.R.Int64N(2000) - 1000
	case 3:
		return m.R.NormFloat64() * 1e3
	case 4, 5:
		return []string{"", "OrderPaid", "EUR", "jo@x.io", "日本語", "line\nbreak", "tab\t\"q\"", "4111111111111111", "2026-01-01T00:00:00Z"}[m.R.IntN(9)]
	case 6:
		return []any{m.random(depth + 1), m.random(depth + 1)}
	}
	return map[string]any{m.key(): m.random(depth + 1), m.key(): m.random(depth + 1)}
}

// write renders v with random key order, spacing, duplicates, escapes and number forms.
func (m EventMutator) write(b *bytes.Buffer, v any, depth int) {
	switch t := v.(type) {
	case map[string]any:
		m.object(b, t, depth)
	case []any:
		m.array(b, t, depth)
	case string:
		m.text(b, t)
	case int64:
		b.WriteString(m.integer(t))
	case float64:
		b.WriteString(m.float(t))
	default:
		out, _ := codec.TreeWriter{}.Write(v)
		b.Write(out)
	}
}

func (m EventMutator) object(b *bytes.Buffer, o map[string]any, depth int) {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	m.R.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	b.WriteString("{" + m.space())
	for i, k := range keys {
		if i > 0 {
			b.WriteString(m.space() + "," + m.space())
		}
		if m.odds(rateDuplicate) {
			m.text(b, k)
			b.WriteString(":")
			m.write(b, m.random(depth+1), depth+1)
			b.WriteString(",")
		}
		m.text(b, k)
		b.WriteString(m.space() + ":" + m.space())
		m.write(b, o[k], depth+1)
	}
	b.WriteString(m.space() + "}")
}

func (m EventMutator) array(b *bytes.Buffer, a []any, depth int) {
	b.WriteString("[" + m.space())
	for i, e := range a {
		if i > 0 {
			b.WriteString("," + m.space())
		}
		m.write(b, e, depth+1)
	}
	b.WriteString(m.space() + "]")
}

func (m EventMutator) space() string {
	if !m.odds(rateSpace) {
		return ""
	}
	return []string{" ", "\n", "\t", "\r\n  "}[m.R.IntN(4)]
}

func (m EventMutator) text(b *bytes.Buffer, s string) {
	if !m.odds(rateEscape) {
		out, _ := codec.TreeWriter{}.Write(s)
		b.Write(out)
		return
	}
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteString(`\` + string(r))
		case r < 0x20 || m.R.IntN(2) == 0:
			m.escape(b, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// escape writes r as \uXXXX (a surrogate pair above the BMP).
func (EventMutator) escape(b *bytes.Buffer, r rune) {
	if r1, r2 := utf16.EncodeRune(r); r1 != unicode.ReplacementChar {
		fmt.Fprintf(b, `\u%04x\u%04x`, r1, r2)
		return
	}
	fmt.Fprintf(b, `\u%04x`, r)
}

func (m EventMutator) integer(i int64) string {
	if !m.odds(rateNumber) {
		return strconv.FormatInt(i, 10)
	}
	return []string{strconv.FormatInt(i, 10) + ".0", strconv.FormatInt(i, 10) + "e0", "12345678901234567890", "-0", "1e400"}[m.R.IntN(5)]
}

func (m EventMutator) float(f float64) string {
	if !m.odds(rateNumber) {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return []string{strconv.FormatFloat(f, 'e', -1, 64), strconv.FormatFloat(f, 'E', 3, 64), strconv.FormatFloat(f, 'f', 8, 64), "0.0000001"}[m.R.IntN(4)]
}
