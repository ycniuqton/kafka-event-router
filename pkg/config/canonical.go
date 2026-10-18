package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// ConfigHash is the hex SHA-256 of a document's canonical JSON (route_versions.config_hash).
type ConfigHash string

// Canonicalizer produces RFC 8785 (JCS) canonical JSON so equal configs hash equally.
type Canonicalizer struct{}

// Canonical returns the canonical JSON bytes of raw.
func (Canonicalizer) Canonical(raw []byte) ([]byte, error) {
	tree, err := JSONDecoder{}.Decode(raw)
	if err != nil {
		return nil, err
	}
	w := &jcsWriter{}
	if err := w.value(tree); err != nil {
		return nil, err
	}
	return w.buf.Bytes(), nil
}

// Hash returns the ConfigHash of raw.
func (c Canonicalizer) Hash(raw []byte) (ConfigHash, error) {
	canon, err := c.Canonical(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return ConfigHash(hex.EncodeToString(sum[:])), nil
}

// jcsWriter serialises a generic JSON tree in JCS form.
type jcsWriter struct {
	buf bytes.Buffer
}

func (w *jcsWriter) value(v any) error {
	switch t := v.(type) {
	case nil:
		w.buf.WriteString("null")
	case bool:
		w.buf.WriteString(strconv.FormatBool(t))
	case string:
		w.str(t)
	case json.Number:
		return w.number(t)
	case []any:
		return w.array(t)
	case map[string]any:
		return w.object(t)
	default:
		return fmt.Errorf("canonical json: unsupported value %T", v)
	}
	return nil
}

func (w *jcsWriter) array(a []any) error {
	w.buf.WriteByte('[')
	for i, e := range a {
		if i > 0 {
			w.buf.WriteByte(',')
		}
		if err := w.value(e); err != nil {
			return err
		}
	}
	w.buf.WriteByte(']')
	return nil
}

func (w *jcsWriter) object(m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return w.less(keys[i], keys[j]) })
	w.buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			w.buf.WriteByte(',')
		}
		w.str(k)
		w.buf.WriteByte(':')
		if err := w.value(m[k]); err != nil {
			return err
		}
	}
	w.buf.WriteByte('}')
	return nil
}

// str writes a JCS string: only quote, backslash and control characters are escaped.
func (w *jcsWriter) str(s string) {
	w.buf.WriteByte('"')
	for _, r := range s {
		w.rune(r)
	}
	w.buf.WriteByte('"')
}

func (w *jcsWriter) rune(r rune) {
	switch r {
	case '"':
		w.buf.WriteString(`\"`)
	case '\\':
		w.buf.WriteString(`\\`)
	case '\b':
		w.buf.WriteString(`\b`)
	case '\f':
		w.buf.WriteString(`\f`)
	case '\n':
		w.buf.WriteString(`\n`)
	case '\r':
		w.buf.WriteString(`\r`)
	case '\t':
		w.buf.WriteString(`\t`)
	default:
		if r < 0x20 {
			fmt.Fprintf(&w.buf, `\u%04x`, r)
			return
		}
		w.buf.WriteRune(r)
	}
}

// number writes the ECMAScript Number.prototype.toString form of the IEEE double.
func (w *jcsWriter) number(n json.Number) error {
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Errorf("canonical json: invalid number %q", n)
	}
	w.buf.WriteString(esNumber{}.format(f))
	return nil
}

// esNumber formats doubles like ECMAScript (shortest round-trip digits).
type esNumber struct{}

// esExponentLimit bounds the decimal exponent range written without exponent notation.
type esExponentLimit int

const (
	esMaxFixedExponent esExponentLimit = 21
	esMinFixedExponent esExponentLimit = -6
)

func (esNumber) format(f float64) string {
	if f == 0 {
		return "0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	sci := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expStr, _ := strings.Cut(sci, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expStr)
	return sign + esNumber{}.layout(digits, exp+1)
}

// layout places the decimal point for digits d with point position n.
func (esNumber) layout(d string, n int) string {
	k := len(d)
	switch {
	case k <= n && n <= int(esMaxFixedExponent):
		return d + strings.Repeat("0", n-k)
	case 0 < n && n <= int(esMaxFixedExponent):
		return d[:n] + "." + d[n:]
	case int(esMinFixedExponent) < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + d
	}
	frac := ""
	if k > 1 {
		frac = "." + d[1:]
	}
	expSign := "+"
	if n-1 < 0 {
		expSign = "-"
	}
	return d[:1] + frac + "e" + expSign + strconv.Itoa(esNumber{}.abs(n-1))
}

func (esNumber) abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// less compares member names by UTF-16 code units as JCS requires.
func (*jcsWriter) less(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}
