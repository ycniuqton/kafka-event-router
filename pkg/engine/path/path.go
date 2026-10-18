// Package path parses field paths such as `value.order.items[0].price` or
// `headers.traceId` once at compile time and reads/writes them on events.
package path

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PathRoot is the event member a field path starts from.
type PathRoot string

const (
	RootKey       PathRoot = "key"
	RootValue     PathRoot = "value"
	RootHeaders   PathRoot = "headers"
	RootTopic     PathRoot = "topic"
	RootPartition PathRoot = "partition"
	RootOffset    PathRoot = "offset"
	RootTimestamp PathRoot = "timestamp"
)

// Syntax holds the path grammar delimiters.
type Syntax string

const (
	SyntaxDot          Syntax = "."
	SyntaxIndexOpen    Syntax = "["
	SyntaxIndexClose   Syntax = "]"
	SyntaxSegmentBreak Syntax = ".["
)

// ErrSyntax reports a malformed path.
var ErrSyntax = errors.New("invalid field path")

// Segment is one object member or array index.
type Segment struct {
	Name    string
	Index   int
	IsIndex bool
}

// FieldPath is a parsed path; it doubles as the Accessor of that path.
type FieldPath struct {
	Root     PathRoot
	Segments []Segment
	raw      string
}

// String returns the path as written.
func (p FieldPath) String() string { return p.raw }

// Parser parses field path strings.
type Parser struct{}

// Parse validates the root and the segment grammar.
func (Parser) Parse(s string) (FieldPath, error) {
	rootName, rest := Parser{}.splitRoot(s)
	root := PathRoot(rootName)
	rule, ok := rootRules[root]
	if !ok {
		return FieldPath{}, fmt.Errorf("%w %q: unknown root %q", ErrSyntax, s, rootName)
	}
	segs, err := segmentScanner{src: rest}.scan()
	if err != nil {
		return FieldPath{}, fmt.Errorf("%w %q: %v", ErrSyntax, s, err)
	}
	if err := rule.check(segs); err != nil {
		return FieldPath{}, fmt.Errorf("%w %q: %v", ErrSyntax, s, err)
	}
	return FieldPath{Root: root, Segments: segs, raw: s}, nil
}

// MustParse parses or panics; for tests and static tables only.
func (Parser) MustParse(s string) FieldPath {
	p, err := Parser{}.Parse(s)
	if err != nil {
		panic(err)
	}
	return p
}

func (Parser) splitRoot(s string) (string, string) {
	i := strings.IndexAny(s, string(SyntaxSegmentBreak))
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

// segmentScanner turns ".a[0].b" into segments.
type segmentScanner struct {
	src string
	pos int
}

func (sc segmentScanner) scan() ([]Segment, error) {
	segs := []Segment{}
	for sc.pos < len(sc.src) {
		seg, err := sc.next()
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
	}
	return segs, nil
}

func (sc *segmentScanner) next() (Segment, error) {
	if strings.HasPrefix(sc.src[sc.pos:], string(SyntaxIndexOpen)) {
		return sc.index()
	}
	if !strings.HasPrefix(sc.src[sc.pos:], string(SyntaxDot)) {
		return Segment{}, fmt.Errorf("unexpected %q at %d", sc.src[sc.pos], sc.pos)
	}
	sc.pos++
	end := strings.IndexAny(sc.src[sc.pos:], string(SyntaxSegmentBreak)+string(SyntaxIndexClose))
	if end < 0 {
		end = len(sc.src) - sc.pos
	}
	name := sc.src[sc.pos : sc.pos+end]
	if name == "" {
		return Segment{}, fmt.Errorf("empty segment at %d", sc.pos)
	}
	sc.pos += end
	return Segment{Name: name}, nil
}

func (sc *segmentScanner) index() (Segment, error) {
	end := strings.Index(sc.src[sc.pos:], string(SyntaxIndexClose))
	if end < 0 {
		return Segment{}, fmt.Errorf("unclosed index at %d", sc.pos)
	}
	body := sc.src[sc.pos+1 : sc.pos+end]
	i, err := strconv.Atoi(body)
	if err != nil || i < 0 || strconv.Itoa(i) != body {
		return Segment{}, fmt.Errorf("index %q is not a non-negative integer", body)
	}
	sc.pos += end + 1
	return Segment{Index: i, IsIndex: true}, nil
}

// rootRule constrains the segments allowed after a root.
type rootRule struct {
	minSegments int
	maxSegments int
	namesOnly   bool
	writable    bool
}

// unbounded marks a rule without a segment maximum.
const unbounded = -1

var rootRules = map[PathRoot]rootRule{
	RootKey:       {minSegments: 0, maxSegments: unbounded, writable: true},
	RootValue:     {minSegments: 0, maxSegments: unbounded, writable: true},
	RootHeaders:   {minSegments: 1, maxSegments: 1, namesOnly: true, writable: true},
	RootTopic:     {},
	RootPartition: {},
	RootOffset:    {},
	RootTimestamp: {},
}

func (r rootRule) check(segs []Segment) error {
	if len(segs) < r.minSegments {
		return fmt.Errorf("needs at least %d segment(s)", r.minSegments)
	}
	if r.maxSegments != unbounded && len(segs) > r.maxSegments {
		return fmt.Errorf("allows at most %d segment(s)", r.maxSegments)
	}
	if r.namesOnly && len(segs) > 0 && segs[0].IsIndex {
		return errors.New("expects a name, not an index")
	}
	return nil
}

// Build constructs a path from a root and segments without parsing, so member names may
// contain grammar characters (header names, mapper keys); String renders the dotted form.
func (Parser) Build(root PathRoot, segs ...Segment) FieldPath {
	var b strings.Builder
	b.WriteString(string(root))
	for _, s := range segs {
		if s.IsIndex {
			b.WriteString(string(SyntaxIndexOpen) + strconv.Itoa(s.Index) + string(SyntaxIndexClose))
			continue
		}
		b.WriteString(string(SyntaxDot) + s.Name)
	}
	return FieldPath{Root: root, Segments: append([]Segment(nil), segs...), raw: b.String()}
}

// Child returns the path extended by one member name.
func (p FieldPath) Child(name string) FieldPath {
	segs := append(append([]Segment(nil), p.Segments...), Segment{Name: name})
	return Parser{}.Build(p.Root, segs...)
}

// RootOnly returns the path of the root member itself (the whole key or value).
func (p FieldPath) RootOnly() FieldPath { return Parser{}.Build(p.Root) }
