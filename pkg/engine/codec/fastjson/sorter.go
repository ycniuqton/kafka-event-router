package fastjson

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"slices"
)

// KeyRef is a pointer-free sort entry of one object member: its name lives in the raw
// input (unescaped names) or in the writer's arena (decoded names).
type KeyRef struct {
	Prefix     uint64
	Start, End int32
	Val        int32
	Idx        int32
	Arena      bool
}

// PrefixOf packs the first 8 name bytes big-endian (zero padded), so most comparisons
// are one integer compare.
func PrefixOf(b []byte) uint64 {
	if len(b) >= int(swarWidth) {
		return binary.BigEndian.Uint64(b)
	}
	var pad [swarWidth]byte
	copy(pad[:], b)
	return binary.BigEndian.Uint64(pad[:])
}

// sortLevel is the reusable sort state of one nesting depth; perm is the member order of
// the last full sort, tried first on the next object (arrays of same-shaped objects).
type sortLevel struct {
	refs    []KeyRef
	ordered []KeyRef
	perm    []int32
}

// MemberSorter orders object members by name (last duplicate wins) for one writer.
type MemberSorter struct {
	levels []sortLevel
}

// Sort returns the members of obj in output order.
func (s *MemberSorter) Sort(w *CanonicalWriter, obj int32, depth int) []KeyRef {
	for len(s.levels) <= depth {
		s.levels = append(s.levels, sortLevel{})
	}
	lv := &s.levels[depth]
	lv.refs = lv.refs[:0]
	it := w.T.MembersOf(obj)
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		r := w.ref(k, v)
		r.Idx = int32(len(lv.refs))
		lv.refs = append(lv.refs, r)
	}
	if w.T.Entries[obj].Sorted() {
		return lv.refs
	}
	if out, ok := s.reuse(w, lv); ok {
		return out
	}
	return s.full(w, lv)
}

// reuse applies the previous order and keeps it when the result strictly ascends.
func (s *MemberSorter) reuse(w *CanonicalWriter, lv *sortLevel) ([]KeyRef, bool) {
	if len(lv.perm) != len(lv.refs) {
		return nil, false
	}
	lv.ordered = lv.ordered[:0]
	for i, p := range lv.perm {
		lv.ordered = append(lv.ordered, lv.refs[p])
		if i > 0 && w.compareNames(lv.ordered[i-1], lv.ordered[i]) >= 0 {
			return nil, false
		}
	}
	return lv.ordered, true
}

// full sorts, drops earlier duplicates and remembers the order when there were none.
func (s *MemberSorter) full(w *CanonicalWriter, lv *sortLevel) []KeyRef {
	slices.SortFunc(lv.refs, w.compare)
	n := len(lv.refs)
	out := lv.refs[:0]
	for i, r := range lv.refs {
		if i+1 < n && w.compareNames(r, lv.refs[i+1]) == 0 {
			continue
		}
		out = append(out, r)
	}
	lv.perm = lv.perm[:0]
	if len(out) == n {
		for _, r := range out {
			lv.perm = append(lv.perm, r.Idx)
		}
	}
	return out
}

// compareNames orders refs by name only.
func (w *CanonicalWriter) compareNames(a, b KeyRef) int {
	if a.Prefix != b.Prefix {
		return cmp.Compare(a.Prefix, b.Prefix)
	}
	return bytes.Compare(w.name(a), w.name(b))
}

// compare orders refs by name, then by position (the last duplicate sorts last).
func (w *CanonicalWriter) compare(a, b KeyRef) int {
	if c := w.compareNames(a, b); c != 0 {
		return c
	}
	return cmp.Compare(a.Val, b.Val)
}
