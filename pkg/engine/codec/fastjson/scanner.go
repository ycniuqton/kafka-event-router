package fastjson

import (
	"math"
	"unicode/utf8"
)

// Limit bounds what the scanner accepts; larger inputs take the tree path.
type Limit int

const (
	// MaxDepth is the deepest nesting scanned (the tree reader allows 10000).
	MaxDepth Limit = 512
	// MaxBytes keeps every offset within int32.
	MaxBytes Limit = math.MaxInt32
	// MinEntries is the tape growth step.
	MinEntries Limit = 64
)

// Grammar holds the JSON delimiter and literal bytes the scanner matches.
type Grammar byte

const (
	ObjectOpen  Grammar = '{'
	ObjectClose Grammar = '}'
	ArrayOpen   Grammar = '['
	ArrayClose  Grammar = ']'
	Quote       Grammar = '"'
	Colon       Grammar = ':'
	Comma       Grammar = ','
	Backslash   Grammar = '\\'
	Minus       Grammar = '-'
	Plus        Grammar = '+'
	Dot         Grammar = '.'
	Zero        Grammar = '0'
	Nine        Grammar = '9'
	ControlEnd  Grammar = 0x20
	ASCIIEnd    Grammar = 0x80
	CaseBit     Grammar = 0x20
	ExponentLow Grammar = 'e'
	UnicodeEsc  Grammar = 'u'
	HexDigits   Grammar = 4
)

// Literal is a JSON literal token.
type Literal string

const (
	LiteralTrue  Literal = "true"
	LiteralFalse Literal = "false"
	LiteralNull  Literal = "null"
)

// Scanner turns bytes into a tape in one pass; Scan reports false for invalid JSON and
// for inputs outside the limits, which callers hand to the tree reader instead.
type Scanner struct {
	raw     []byte
	pos     int
	depth   int
	n       int
	entries []Entry
}

// Scan fills t from raw and reports whether raw is one valid JSON value.
func (s *Scanner) Scan(raw []byte, t *Tape) bool {
	if len(raw) > int(MaxBytes) {
		return false
	}
	*s = Scanner{raw: raw, entries: t.Entries[:cap(t.Entries)]}
	t.Raw, t.Canonical = raw, false
	lead := s.skipSpace()
	_, ok := s.value()
	trail := s.skipSpace()
	t.Entries, s.entries = s.entries[:s.n], nil
	if !ok || s.pos != len(raw) {
		return false
	}
	t.Canonical = !lead && !trail && t.Entries[Root].Canonical()
	return true
}

// value scans one value of any kind and returns its tape index.
func (s *Scanner) value() (int32, bool) {
	if s.pos >= len(s.raw) {
		return 0, false
	}
	switch c := s.raw[s.pos]; {
	case c == byte(ObjectOpen):
		return s.object()
	case c == byte(ArrayOpen):
		return s.array()
	case c == byte(Quote):
		return s.str()
	case c == byte(Minus) || (c >= byte(Zero) && c <= byte(Nine)):
		return s.number()
	}
	return s.literal()
}

// push appends an entry that starts at the current position.
func (s *Scanner) push(k Kind) int32 {
	if s.n == len(s.entries) {
		s.entries = append(s.entries, make([]Entry, len(s.entries)+int(MinEntries))...)
	}
	i := s.n
	s.entries[i] = Entry{Start: int32(s.pos), Kind: k}
	s.n++
	return int32(i)
}

// finish closes entry i at the current position.
func (s *Scanner) finish(i int32, f Flags) int32 {
	e := &s.entries[i]
	e.End, e.Next, e.Flags = int32(s.pos), int32(s.n), f
	return i
}

// canonicalIf returns FlagCanonical when ok.
func (Scanner) canonicalIf(ok bool) Flags {
	if ok {
		return FlagCanonical
	}
	return 0
}

// skipSpace skips insignificant whitespace and reports whether there was any.
func (s *Scanner) skipSpace() bool {
	raw, pos := s.raw, s.pos
	if pos >= len(raw) || !whitespace[raw[pos]] {
		return false
	}
	for pos < len(raw) && whitespace[raw[pos]] {
		pos++
	}
	moved := pos != s.pos
	s.pos = pos
	return moved
}

// expect consumes byte g after optional whitespace and reports (found, whitespace seen).
func (s *Scanner) expect(g Grammar) (bool, bool) {
	ws := s.skipSpace()
	if s.pos >= len(s.raw) || s.raw[s.pos] != byte(g) {
		return false, ws
	}
	s.pos++
	return true, ws
}

// enter and leave track nesting against MaxDepth.
func (s *Scanner) enter() bool {
	s.depth++
	return s.depth <= int(MaxDepth)
}

func (s *Scanner) leave() { s.depth-- }

// literal scans true, false or null.
func (s *Scanner) literal() (int32, bool) {
	rest := s.raw[s.pos:]
	for _, l := range literals {
		if len(rest) >= len(l.text) && string(rest[:len(l.text)]) == string(l.text) {
			i := s.push(l.kind)
			s.pos += len(l.text)
			return s.finish(i, FlagCanonical), true
		}
	}
	return 0, false
}

// literalKind pairs a literal with its kind.
type literalKind struct {
	text Literal
	kind Kind
}

var literals = []literalKind{{LiteralTrue, KindTrue}, {LiteralFalse, KindFalse}, {LiteralNull, KindNull}}

// whitespace holds the insignificant whitespace bytes of RFC 8259.
var whitespace = [256]bool{' ': true, '\t': true, '\n': true, '\r': true}

// UTF8 validates string bodies that contain non-ASCII bytes.
type UTF8 struct{}

// Valid reports whether b is valid UTF-8.
func (UTF8) Valid(b []byte) bool { return utf8.Valid(b) }
