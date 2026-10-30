package fastjson

import (
	"bytes"
	"encoding/binary"
)

// object scans {...}; it is canonical when compact, its keys strictly ascend and every
// member is canonical.
func (s *Scanner) object() (int32, bool) {
	i := s.push(KindObject)
	s.pos++
	if !s.enter() {
		return 0, false
	}
	defer s.leave()
	done, lead := s.expect(ObjectClose)
	if done {
		return s.finish(i, s.canonicalIf(!lead)|FlagSorted), true
	}
	canon, sorted, prev := !lead, true, int32(-1)
	for {
		key, ok, mc := s.member()
		if !ok {
			return 0, false
		}
		sorted = sorted && !s.entries[key].Escaped() && s.ascending(prev, key)
		canon = canon && mc && sorted
		prev = key
		more, ok, ws := s.separator(ObjectClose)
		if !ok {
			return 0, false
		}
		canon = canon && !ws
		if !more {
			return s.finish(i, s.canonicalIf(canon)|s.sortedIf(sorted)), true
		}
	}
}

// sortedIf returns FlagSorted when ok.
func (Scanner) sortedIf(ok bool) Flags {
	if ok {
		return FlagSorted
	}
	return 0
}

// member scans `"key": value` and reports whether it was compact and canonical.
func (s *Scanner) member() (int32, bool, bool) {
	ws := s.skipSpace()
	if s.pos >= len(s.raw) || s.raw[s.pos] != byte(Quote) {
		return 0, false, false
	}
	key, ok := s.str()
	if !ok {
		return 0, false, false
	}
	found, ws2 := s.expect(Colon)
	if !found {
		return 0, false, false
	}
	ws3 := s.skipSpace()
	val, ok := s.value()
	if !ok {
		return 0, false, false
	}
	canon := !ws && !ws2 && !ws3 && s.entries[key].Canonical() && s.entries[val].Canonical()
	return key, true, canon
}

// ascending reports whether key sorts strictly after prev (raw comparison of unescaped keys).
func (s *Scanner) ascending(prev, key int32) bool {
	if prev < 0 {
		return true
	}
	a, b := s.entries[prev], s.entries[key]
	if a.Escaped() {
		return false
	}
	return bytes.Compare(s.raw[a.Start+1:a.End-1], s.raw[b.Start+1:b.End-1]) < 0
}

// separator consumes `,` (more) or the closing byte; ws reports whitespace before it.
func (s *Scanner) separator(closing Grammar) (more, ok, ws bool) {
	ws = s.skipSpace()
	if s.pos >= len(s.raw) {
		return false, false, ws
	}
	switch s.raw[s.pos] {
	case byte(Comma):
		s.pos++
		return true, true, ws
	case byte(closing):
		s.pos++
		return false, true, ws
	}
	return false, false, ws
}

// array scans [...]; it is canonical when compact and every element is canonical.
func (s *Scanner) array() (int32, bool) {
	i := s.push(KindArray)
	s.pos++
	if !s.enter() {
		return 0, false
	}
	defer s.leave()
	done, lead := s.expect(ArrayClose)
	if done {
		return s.finish(i, s.canonicalIf(!lead)), true
	}
	canon := !lead
	for {
		ws := s.skipSpace()
		el, ok := s.value()
		if !ok {
			return 0, false
		}
		more, ok, ws2 := s.separator(ArrayClose)
		if !ok {
			return 0, false
		}
		canon = canon && !ws && !ws2 && s.entries[el].Canonical()
		if !more {
			return s.finish(i, s.canonicalIf(canon)), true
		}
	}
}

// str scans a string literal; plain valid UTF-8 without escapes is canonical.
func (s *Scanner) str() (int32, bool) {
	i := s.push(KindString)
	s.pos++
	escaped, high := false, false
	for {
		s.plainRun()
		if s.pos >= len(s.raw) {
			return 0, false
		}
		switch c := s.raw[s.pos]; {
		case c == byte(Quote):
			s.pos++
			return s.finish(i, s.stringFlags(i, escaped, high)), true
		case c == byte(Backslash):
			if !s.escape() {
				return 0, false
			}
			escaped = true
		case c < byte(ControlEnd):
			return 0, false
		default:
			high = true
			s.pos++
		}
	}
}

// plainRun skips string bytes that need no attention.
func (s *Scanner) plainRun() {
	raw, pos := s.raw, s.pos
	for pos+int(swarWidth) <= len(raw) && !(SWAR{}).special(binary.LittleEndian.Uint64(raw[pos:])) {
		pos += int(swarWidth)
	}
	for pos < len(raw) && stringPlain[raw[pos]] {
		pos++
	}
	s.pos = pos
}

// SWAR tests eight string bytes at once.
type SWAR struct{}

// swarWidth is the number of bytes tested at once.
const swarWidth Limit = 8

// SWAR byte masks.
const (
	swarOnes    uint64 = 0x0101010101010101
	swarHigh    uint64 = 0x8080808080808080
	swarQuote   uint64 = 0x2222222222222222
	swarSlash   uint64 = 0x5c5c5c5c5c5c5c5c
	swarControl uint64 = 0x2020202020202020
)

// special reports whether any byte of x may be a quote, a backslash, a control byte or
// non-ASCII (false positives only slow the scan down).
func (SWAR) special(x uint64) bool {
	q := x ^ swarQuote
	b := x ^ swarSlash
	t := (q - swarOnes) & ^q
	t |= (b - swarOnes) & ^b
	t |= x - swarControl
	return (t|x)&swarHigh != 0
}

// stringFlags classifies a scanned string.
func (s *Scanner) stringFlags(i int32, escaped, high bool) Flags {
	if !escaped && high {
		e := s.entries[i]
		escaped = !(UTF8{}).Valid(s.raw[e.Start+1 : s.pos-1])
	}
	if escaped {
		return FlagEscaped
	}
	return FlagCanonical
}

// escape consumes one backslash escape sequence.
func (s *Scanner) escape() bool {
	if s.pos+1 >= len(s.raw) {
		return false
	}
	c := s.raw[s.pos+1]
	if simpleEscape[c] {
		s.pos += 2
		return true
	}
	end := s.pos + 2 + int(HexDigits)
	if c != byte(UnicodeEsc) || end > len(s.raw) {
		return false
	}
	for _, h := range s.raw[s.pos+2 : end] {
		if !hexDigit[h] {
			return false
		}
	}
	s.pos = end
	return true
}

// number scans an RFC 8259 number and flags it canonical when the tree writer would
// print the same text.
func (s *Scanner) number() (int32, bool) {
	i := s.push(KindNumber)
	start := s.pos
	if s.raw[s.pos] == byte(Minus) {
		s.pos++
	}
	if !s.integerPart() {
		return 0, false
	}
	frac, ok := s.part(byte(Dot), false)
	if !ok {
		return 0, false
	}
	exp, ok := s.part(byte(ExponentLow), true)
	if !ok {
		return 0, false
	}
	canon := NumberText{}.Canonical(s.raw[start:s.pos], frac, exp)
	return s.finish(i, s.canonicalIf(canon)), true
}

// integerPart scans `0` or a non-zero digit run.
func (s *Scanner) integerPart() bool {
	if s.pos >= len(s.raw) || !decimalDigit[s.raw[s.pos]] {
		return false
	}
	if s.raw[s.pos] == byte(Zero) {
		s.pos++
		return true
	}
	s.digits()
	return true
}

// part scans an optional fraction (`.` digits) or exponent (`e`/`E`, sign, digits).
func (s *Scanner) part(marker byte, exponent bool) (present, ok bool) {
	if s.pos >= len(s.raw) || (s.raw[s.pos]|exponentCase(exponent)) != marker {
		return false, true
	}
	s.pos++
	if exponent && s.pos < len(s.raw) && (s.raw[s.pos] == byte(Plus) || s.raw[s.pos] == byte(Minus)) {
		s.pos++
	}
	if s.pos >= len(s.raw) || !decimalDigit[s.raw[s.pos]] {
		return true, false
	}
	s.digits()
	return true, true
}

// exponentCase folds `E` to `e` for the exponent marker only.
func exponentCase(exponent bool) byte {
	if exponent {
		return byte(CaseBit)
	}
	return 0
}

// digits consumes a decimal digit run.
func (s *Scanner) digits() {
	raw, pos := s.raw, s.pos
	for pos < len(raw) && decimalDigit[raw[pos]] {
		pos++
	}
	s.pos = pos
}

// Byte classes of the scanner.
var (
	stringPlain = byteClass(func(c byte) bool {
		return c >= byte(ControlEnd) && c < byte(ASCIIEnd) && c != byte(Quote) && c != byte(Backslash)
	})
	simpleEscape = byteClass(func(c byte) bool { return bytes.IndexByte([]byte(`"\/bfnrt`), c) >= 0 })
	hexDigit     = byteClass(func(c byte) bool { return bytes.IndexByte([]byte("0123456789abcdefABCDEF"), c) >= 0 })
	decimalDigit = byteClass(func(c byte) bool { return c >= byte(Zero) && c <= byte(Nine) })
)

// byteClass tabulates a byte predicate.
func byteClass(in func(byte) bool) [256]bool {
	var t [256]bool
	for c := 0; c < 256; c++ {
		t[c] = in(byte(c))
	}
	return t
}
