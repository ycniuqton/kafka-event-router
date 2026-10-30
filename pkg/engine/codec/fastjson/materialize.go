package fastjson

// Materializer builds Go values from tape entries, identical to codec.TreeReader:
// map[string]any (duplicate names: last wins), []any, string, int64 / float64 /
// json.Number, bool and nil.
type Materializer struct {
	T *Tape
}

// Value returns the Go value of entry i.
func (m Materializer) Value(i int32) any {
	e := m.T.Entries[i]
	switch e.Kind {
	case KindObject:
		return m.object(i)
	case KindArray:
		return m.array(i)
	case KindString:
		return m.String(i)
	case KindNumber:
		return NumberText{}.Value(m.T.Raw[e.Start:e.End])
	case KindTrue:
		return true
	case KindFalse:
		return false
	}
	return nil
}

// String returns an owned copy of the decoded text of string entry i.
func (m Materializer) String(i int32) string {
	e := m.T.Entries[i]
	if e.Escaped() {
		return Strings{}.Decode(m.T.Raw[e.Start:e.End])
	}
	return string(m.T.Raw[e.Start+1 : e.End-1])
}

func (m Materializer) object(i int32) map[string]any {
	out := map[string]any{}
	it := m.T.MembersOf(i)
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		out[m.String(k)] = m.Value(v)
	}
	return out
}

func (m Materializer) array(i int32) []any {
	out := []any{}
	it := m.T.ElementsOf(i)
	for el, ok := it.Next(); ok; el, ok = it.Next() {
		out = append(out, m.Value(el))
	}
	return out
}
