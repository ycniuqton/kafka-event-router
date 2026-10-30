package fastjson

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"strconv"
	"unsafe"
)

// Strings decodes and quotes JSON strings exactly like the jsontext decoder and encoder
// used by the tree codec (invalid UTF-8 and lone surrogates become U+FFFD, no HTML escaping).
type Strings struct{}

// Decode returns the text of a quoted JSON string literal.
func (Strings) Decode(quoted []byte) string {
	out, _ := jsontext.AppendUnquote(nil, quoted)
	return string(out)
}

// Append quotes s onto dst in canonical (shortest) form.
func (Strings) Append(dst []byte, s string) []byte {
	out, _ := jsontext.AppendQuote(dst, s)
	return out
}

// AppendRaw re-quotes a scanned string literal onto dst in canonical form.
func (Strings) AppendRaw(dst []byte, quoted []byte, scratch *[]byte) []byte {
	buf, _ := jsontext.AppendUnquote((*scratch)[:0], quoted)
	*scratch = buf
	out, _ := jsontext.AppendQuote(dst, buf)
	return out
}

// NumberBound holds the limits of the canonical-number shortcuts.
type NumberBound int

const (
	// SafeIntDigits always fit int64.
	SafeIntDigits NumberBound = 18
	// ExactFloatDigits always print back unchanged as the shortest float64 form.
	ExactFloatDigits NumberBound = 15
	// MaxFixedLeadingZeros keeps fractions at or above 1e-6 (fixed, not exponent, notation).
	MaxFixedLeadingZeros NumberBound = 5
)

// NumberText converts JSON number text the way the tree codec does: exact integers to
// int64, other finite numbers to float64, out-of-range numbers kept as json.Number.
type NumberText struct{}

// integral reports whether text has neither fraction nor exponent; ParseInt rejects the
// others anyway, so skipping it there only saves its error allocation.
func (NumberText) integral(text []byte) bool {
	return bytes.IndexAny(text, ".eE") < 0
}

// Value returns the Go value of number text.
func (n NumberText) Value(text []byte) any {
	s := unsafe.String(unsafe.SliceData(text), len(text))
	if !n.integral(text) {
		return n.float(text, s)
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	return n.float(text, s)
}

// float is the non-int64 part of Value.
func (NumberText) float(text []byte, s string) any {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return json.Number(string(text))
}

// Append writes the tree writer's text for number text.
func (n NumberText) Append(dst []byte, text []byte) []byte {
	s := unsafe.String(unsafe.SliceData(text), len(text))
	if n.integral(text) {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return strconv.AppendInt(dst, i, 10)
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return jsontext.AppendFloat(dst, f, 64)
	}
	return append(dst, text...)
}

// Canonical reports, without parsing, whether Append would return text unchanged; it may
// say false for some canonical numbers (they are then re-printed to the same bytes).
func (n NumberText) Canonical(text []byte, frac, exp bool) bool {
	if exp {
		return false
	}
	if !frac {
		return n.canonicalInt(text)
	}
	return n.canonicalDecimal(text)
}

func (NumberText) canonicalInt(text []byte) bool {
	digits := text
	if digits[0] == byte(Minus) {
		digits = digits[1:]
	}
	if len(digits) == 1 && digits[0] == byte(Zero) {
		return len(text) == 1
	}
	if len(digits) <= int(SafeIntDigits) {
		return true
	}
	_, err := strconv.ParseInt(unsafe.String(unsafe.SliceData(text), len(text)), 10, 64)
	return err == nil
}

// canonicalDecimal accepts d.d forms with ≤ 15 significant digits, no trailing zero and
// magnitude ≥ 1e-6: their shortest float64 rendering is the same text.
func (NumberText) canonicalDecimal(text []byte) bool {
	if text[len(text)-1] == byte(Zero) {
		return false
	}
	sig, leading, seenNonZero, afterDot := 0, 0, false, false
	for _, c := range text {
		switch {
		case c == byte(Dot):
			afterDot = true
		case c == byte(Minus):
		case c != byte(Zero) || seenNonZero:
			seenNonZero = true
			sig++
		case afterDot:
			leading++
		}
	}
	return sig <= int(ExactFloatDigits) && leading <= int(MaxFixedLeadingZeros)
}
