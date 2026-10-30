package projection

import (
	"slices"

	"github.com/eventrouter/router/pkg/engine/codec/fastjson"
)

// patch writes object obj with the members of n replaced by cur: original members outside
// the projection are copied canonically, projected members come from cur (absent = removed).
func (d *Document) patch(dst []byte, obj int32, n *FieldNode, cur map[string]any, depth int) ([]byte, error) {
	ms := d.stack.At(depth)
	ms = d.originalMembers(ms, obj, n)
	ms = d.currentMembers(ms, cur, n)
	merged := MemberMerge{}.Of(ms)
	dst = append(dst, byte(fastjson.ObjectOpen))
	first := true
	var err error
	for _, m := range merged {
		if m.Pos != currentPos && m.Ref != nil {
			continue
		}
		if !first {
			dst = append(dst, byte(fastjson.Comma))
		}
		first = false
		dst = append(d.canon.Key(dst, m), byte(fastjson.Colon))
		if dst, err = d.member(dst, m, depth); err != nil {
			return dst, err
		}
	}
	d.stack.Keep(depth, ms)
	return append(dst, byte(fastjson.ObjectClose)), nil
}

// originalMembers queues every member of obj; Ref marks members inside the projection.
func (d *Document) originalMembers(ms []fastjson.Member, obj int32, n *FieldNode) []fastjson.Member {
	it := d.tape.MembersOf(obj)
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		name := d.tape.KeyView(k)
		m := fastjson.Member{Key: name, Pos: k, Val: v}
		if child := n.Child(name); child != nil {
			m.Ref = child
		}
		ms = append(ms, m)
	}
	return ms
}

// currentMembers queues the members of cur after the originals of the same name.
func (d *Document) currentMembers(ms []fastjson.Member, cur map[string]any, n *FieldNode) []fastjson.Member {
	for k, v := range cur {
		m := fastjson.Member{Key: k, Pos: currentPos, Val: fastjson.NoEntry, Any: v}
		if child := n.Child(k); child != nil {
			m.Ref = child
		}
		ms = append(ms, m)
	}
	return ms
}

// member writes one merged member value.
func (d *Document) member(dst []byte, m fastjson.Member, depth int) ([]byte, error) {
	if m.Pos != currentPos {
		return d.canon.Append(dst, m.Val), nil
	}
	child, _ := m.Ref.(*FieldNode)
	sub, isMap := m.Any.(map[string]any)
	if child.Partial() && isMap && m.Val != fastjson.NoEntry && d.tape.At(m.Val).Kind == fastjson.KindObject {
		return d.patch(dst, m.Val, child, sub, depth+1)
	}
	return d.values.Append(dst, m.Any)
}

// MemberMerge sorts queued members and keeps one per name: the current member when there
// is one (carrying the tape index of the last original), else the last original.
type MemberMerge struct{}

// Of merges ms in place.
func (MemberMerge) Of(ms []fastjson.Member) []fastjson.Member {
	slices.SortFunc(ms, fastjson.MemberOrder{}.Compare)
	out := ms[:0]
	for i, m := range ms {
		if i+1 < len(ms) && ms[i+1].Key == m.Key {
			ms[i+1].Val = MemberMerge{}.carry(ms[i+1], m.Val)
			continue
		}
		out = append(out, m)
	}
	return out
}

// carry gives a current member the tape index of the original it replaces.
func (MemberMerge) carry(next fastjson.Member, original int32) int32 {
	if next.Pos == currentPos && next.Val == fastjson.NoEntry {
		return original
	}
	return next.Val
}
