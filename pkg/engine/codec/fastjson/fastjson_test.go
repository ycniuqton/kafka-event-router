package fastjson_test

import (
	"bytes"
	"math/rand/v2"
	"reflect"
	"strconv"
	"testing"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/fastjson"
)

// parity checks one input: scanner acceptance, materialized values and canonical bytes
// against the tree reader and writer.
type parity struct {
	tb testing.TB
}

func (p parity) check(raw []byte) {
	p.tb.Helper()
	want, treeErr := codec.TreeReader{}.Read(raw)
	var t fastjson.Tape
	var s fastjson.Scanner
	ok := s.Scan(raw, &t)
	if treeErr != nil {
		if ok {
			p.tb.Fatalf("scanner accepted %q, tree rejected: %v", raw, treeErr)
		}
		return
	}
	if !ok {
		return
	}
	got := fastjson.Materializer{T: &t}.Value(fastjson.Root)
	if !reflect.DeepEqual(got, want) {
		p.tb.Fatalf("materialize %q:\n got %#v\nwant %#v", raw, got, want)
	}
	p.bytes(raw, &t, want)
}

func (p parity) bytes(raw []byte, t *fastjson.Tape, tree any) {
	p.tb.Helper()
	wantBytes, werr := codec.TreeWriter{}.Write(tree)
	if werr != nil {
		p.tb.Fatalf("tree write %q: %v", raw, werr)
	}
	w := fastjson.CanonicalWriter{T: t}
	if got := w.Append(nil, fastjson.Root); !bytes.Equal(got, wantBytes) {
		p.tb.Fatalf("canonical %q:\n got %s\nwant %s", raw, got, wantBytes)
	}
	if t.Canonical && !bytes.Equal(raw, wantBytes) {
		p.tb.Fatalf("flagged canonical but differs %q vs %q", raw, wantBytes)
	}
	vw := fastjson.ValueWriter{Delegate: codec.TreeWriter{}}
	if got, err := vw.Append(nil, tree); err != nil || !bytes.Equal(got, wantBytes) {
		p.tb.Fatalf("value writer %q:\n got %s %v\nwant %s", raw, got, err, wantBytes)
	}
}

// fixedCases cover the edge cases of the tree codec found by probing jsontext.
var fixedCases = []string{
	`{}`, `[]`, `{"a":1}`, `{"b":1,"a":2}`, `{"a":1,"a":2}`, `{"a":{"x":1},"a":[2]}`, ` {"a" : [ 1 , 2 ] } `,
	"{\"a\xff\":1}", `{"\ud800":1}`, `{"\ud800\udc00":"x"}`, `"\udc00"`, `"\ud800x"`, `"a\u2028"`, `"<&>"`,
	`-0`, `-0.0`, `0`, `1e400`, `-1e400`, `12345678901234567890`, `-9223372036854775808`, `9223372036854775807`,
	`9223372036854775808`, `1E2`, `1e+21`, `1e21`, `0.0000001`, `0.000001`, `1.0`, `1.5`, `250.5`, `0.1e1`, `-1e-400`,
	`123456789012345.6`, `12345678901234.56`, `0.30000000000000004`, `100.25`, `-0.5`, "\"\x7f\"", "\xef\xbb\xbf{}",
	`[1,]`, `{"a":}`, `{"a" 1}`, `{,}`, `[01]`, `1.`, `.5`, `-`, `1e`, `1e+`, `tru`, `nul`, `"\x01"`, `"\q"`, `"\u12"`,
	`"\/"`, `"\u0041\n\t\"\\"`, `"é"`, `"\u00e9"`, `{"z":{"b":[{"y":1,"x":2}],"a":null},"a":true}`, `{} {}`, ``, ` `,
	`{"":1,"":2}`, `{"a":"x","A":"y","_":"z","é":1,"0":0}`, `[[[[[]]]]]`, `"`, `{"a":1`, `[1`, `nulll`, `1 2`,
}

func TestCanonicalParityFixed(t *testing.T) {
	for _, c := range fixedCases {
		parity{tb: t}.check([]byte(c))
	}
}

func TestCanonicalParityRandom(t *testing.T) {
	g := Generator{r: rand.New(rand.NewPCG(1, 2))}
	for i := 0; i < 20000; i++ {
		parity{tb: t}.check(g.Document())
	}
}

func TestNumberCanonicalShortcut(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < 200000; i++ {
		text := []byte(Generator{r: r}.number())
		v, err := codec.TreeReader{}.Read(text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		want, _ := codec.TreeWriter{}.Write(v)
		var tape fastjson.Tape
		var s fastjson.Scanner
		if s.Scan(text, &tape) && tape.Canonical && !bytes.Equal(text, want) {
			t.Fatalf("%s flagged canonical, tree writes %s", text, want)
		}
	}
}

func TestScannerDepthLimit(t *testing.T) {
	deep := bytes.Repeat([]byte("["), int(fastjson.MaxDepth)+1)
	deep = append(deep, bytes.Repeat([]byte("]"), int(fastjson.MaxDepth)+1)...)
	var tape fastjson.Tape
	var s fastjson.Scanner
	if s.Scan(deep, &tape) {
		t.Fatal("scanner must hand over-deep documents to the tree reader")
	}
}

func FuzzCanonicalWriter(f *testing.F) {
	for _, c := range fixedCases {
		f.Add([]byte(c))
	}
	g := Generator{r: rand.New(rand.NewPCG(5, 6))}
	for i := 0; i < 50; i++ {
		f.Add(g.Document())
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		parity{tb: t}.check(raw)
	})
}

// Generator produces random JSON documents (nested objects, arrays, unicode, escapes,
// number forms, duplicate keys, whitespace) and occasional corruptions.
type Generator struct {
	r *rand.Rand
}

// Document returns one random document, sometimes invalid.
func (g Generator) Document() []byte {
	doc := []byte(g.value(0))
	if g.r.IntN(10) == 0 && len(doc) > 1 {
		i := g.r.IntN(len(doc))
		doc = append(doc[:i:i], doc[i+1:]...)
	}
	return doc
}

func (g Generator) value(depth int) string {
	n := 7
	if depth > 4 {
		n = 4
	}
	switch g.r.IntN(n) {
	case 0:
		return []string{"null", "true", "false"}[g.r.IntN(3)]
	case 1:
		return g.number()
	case 2, 3:
		return g.str()
	case 4:
		return g.ws() + "[" + g.list(depth, false) + "]"
	}
	return g.ws() + "{" + g.list(depth, true) + "}" + g.ws()
}

func (g Generator) list(depth int, object bool) string {
	var b bytes.Buffer
	for i, n := 0, g.r.IntN(5); i < n; i++ {
		if i > 0 {
			b.WriteString(g.ws() + "," + g.ws())
		}
		if object {
			b.WriteString(g.key() + g.ws() + ":" + g.ws())
		}
		b.WriteString(g.value(depth + 1))
	}
	return b.String()
}

func (g Generator) ws() string {
	if g.r.IntN(4) != 0 {
		return ""
	}
	return []string{" ", "\n", "\t ", "\r\n"}[g.r.IntN(4)]
}

func (g Generator) key() string {
	return []string{`"a"`, `"b"`, `"a"`, `"Z"`, `"é"`, `"\u0061"`, `""`, `"k\n"`, `"id"`, `"x y"`}[g.r.IntN(10)]
}

func (g Generator) str() string {
	parts := []string{"abc", "é", "日本", "\\n", "\\u00e9", "\\ud83d\\ude00", "\\ud800", "\\\"", "\\/", "<&>", "\u2028", " ", "\x7f", "\xff"}
	var b bytes.Buffer
	b.WriteByte('"')
	for i, n := 0, g.r.IntN(4); i < n; i++ {
		b.WriteString(parts[g.r.IntN(len(parts))])
	}
	b.WriteByte('"')
	return b.String()
}

func (g Generator) number() string {
	switch g.r.IntN(8) {
	case 0:
		return strconv.FormatInt(g.r.Int64()-g.r.Int64(), 10)
	case 1:
		return strconv.FormatInt(int64(g.r.IntN(2000))-1000, 10)
	case 2:
		return strconv.FormatFloat(g.r.NormFloat64()*1000, 'f', g.r.IntN(8), 64)
	case 3:
		return strconv.FormatFloat(g.r.ExpFloat64(), 'g', -1, 64)
	case 4:
		return strconv.FormatFloat(g.r.Float64()*1e-5, 'f', -1, 64)
	case 5:
		return []string{"-0", "-0.0", "0.0", "1e400", "12345678901234567890", "1E+2", "1e-7", "100.10", "0.000001"}[g.r.IntN(9)]
	case 6:
		return strconv.FormatInt(g.r.Int64N(1000), 10) + "." + strconv.FormatInt(g.r.Int64N(1000000), 10)
	}
	return strconv.FormatInt(g.r.Int64N(1e15), 10) + "." + strconv.Itoa(g.r.IntN(10))
}
