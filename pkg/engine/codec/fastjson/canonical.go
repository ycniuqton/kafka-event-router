package fastjson

import (
	"cmp"
	"encoding/json/jsontext"
	"slices"
	"strings"
)

// NoEntry marks a member that does not come from the tape.
const NoEntry int32 = -1

// Member is one object member queued for sorted output; Pos orders duplicate names
// (the highest Pos wins, like the last occurrence in the tree reader). Val is the tape
// value index (or NoEntry), Any a Go value and Ref caller data.
type Member struct {
	Key string
	Pos int32
	Val int32
	Any any
	Ref any
}

// MemberOrder sorts members by name, then by position.
type MemberOrder struct{}

// Compare orders a before b.
func (MemberOrder) Compare(a, b Member) int {
	if c := strings.Compare(a.Key, b.Key); c != 0 {
		return c
	}
	return cmp.Compare(a.Pos, b.Pos)
}

// Sort orders ms and drops all but the last member of each name.
func (o MemberOrder) Sort(ms []Member) []Member {
	slices.SortFunc(ms, o.Compare)
	out := ms[:0]
	for i, m := range ms {
		if i+1 < len(ms) && ms[i+1].Key == m.Key {
			continue
		}
		out = append(out, m)
	}
	return out
}

// MemberStack hands out one reusable member slice per nesting depth.
type MemberStack struct {
	levels [][]Member
}

// At returns the empty slice of depth d.
func (s *MemberStack) At(d int) []Member {
	for len(s.levels) <= d {
		s.levels = append(s.levels, nil)
	}
	return s.levels[d][:0]
}

// Keep stores the (possibly grown) slice of depth d for reuse.
func (s *MemberStack) Keep(d int, ms []Member) {
	clear(ms)
	s.levels[d] = ms[:0]
}

// CanonicalWriter writes tape values exactly as codec.TreeWriter writes the tree that
// codec.TreeReader builds from them; canonical subtrees are copied verbatim.
type CanonicalWriter struct {
	T       *Tape
	sorter  MemberSorter
	arena   []byte
	scratch []byte
}

// Reset binds the writer to t.
func (w *CanonicalWriter) Reset(t *Tape) { w.T, w.arena = t, w.arena[:0] }

// Append writes entry i onto dst.
func (w *CanonicalWriter) Append(dst []byte, i int32) []byte {
	return w.value(dst, i, 0)
}

func (w *CanonicalWriter) value(dst []byte, i int32, depth int) []byte {
	e := w.T.Entries[i]
	if e.Canonical() {
		return append(dst, w.T.Raw[e.Start:e.End]...)
	}
	switch e.Kind {
	case KindObject:
		return w.object(dst, i, depth)
	case KindArray:
		return w.array(dst, i, depth)
	case KindString:
		return Strings{}.AppendRaw(dst, w.T.Raw[e.Start:e.End], &w.scratch)
	case KindNumber:
		return NumberText{}.Append(dst, w.T.Raw[e.Start:e.End])
	}
	return append(dst, w.T.Raw[e.Start:e.End]...)
}

// Key writes a member name; unescaped names are copied from the tape.
func (w *CanonicalWriter) Key(dst []byte, m Member) []byte {
	if m.Val != NoEntry && w.T.Entries[m.Val-1].Canonical() {
		k := w.T.Entries[m.Val-1]
		return append(dst, w.T.Raw[k.Start:k.End]...)
	}
	return Strings{}.Append(dst, m.Key)
}

// ref builds the sort entry of the member with key entry k and value entry v.
func (w *CanonicalWriter) ref(k, v int32) KeyRef {
	e := w.T.Entries[k]
	if !e.Escaped() {
		return KeyRef{Prefix: PrefixOf(w.T.Raw[e.Start+1 : e.End-1]), Start: e.Start + 1, End: e.End - 1, Val: v}
	}
	start := int32(len(w.arena))
	w.arena, _ = jsontext.AppendUnquote(w.arena, w.T.Raw[e.Start:e.End])
	return KeyRef{Prefix: PrefixOf(w.arena[start:]), Start: start, End: int32(len(w.arena)), Val: v, Arena: true}
}

// name returns the decoded name bytes of r.
func (w *CanonicalWriter) name(r KeyRef) []byte {
	if r.Arena {
		return w.arena[r.Start:r.End]
	}
	return w.T.Raw[r.Start:r.End]
}

func (w *CanonicalWriter) object(dst []byte, i int32, depth int) []byte {
	dst = append(dst, byte(ObjectOpen))
	for n, r := range w.sorter.Sort(w, i, depth) {
		if n > 0 {
			dst = append(dst, byte(Comma))
		}
		dst = w.key(dst, r)
		dst = w.value(append(dst, byte(Colon)), r.Val, depth+1)
	}
	return append(dst, byte(ObjectClose))
}

// key writes the name of r: raw bytes when canonical, else re-quoted.
func (w *CanonicalWriter) key(dst []byte, r KeyRef) []byte {
	k := w.T.Entries[r.Val-1]
	if k.Canonical() {
		return append(dst, w.T.Raw[k.Start:k.End]...)
	}
	out, _ := jsontext.AppendQuote(dst, w.name(r))
	return out
}

func (w *CanonicalWriter) array(dst []byte, i int32, depth int) []byte {
	dst = append(dst, byte(ArrayOpen))
	it := w.T.ElementsOf(i)
	first := true
	for el, ok := it.Next(); ok; el, ok = it.Next() {
		if !first {
			dst = append(dst, byte(Comma))
		}
		first = false
		dst = w.value(dst, el, depth+1)
	}
	return append(dst, byte(ArrayClose))
}
