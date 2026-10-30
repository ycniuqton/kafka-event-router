package fastjson

import (
	"encoding/json/jsontext"
	"math"
	"slices"
	"strconv"
)

// Delegate encodes values the append writer does not handle itself, with the tree
// writer's exact output and errors (codec.TreeWriter in production).
type Delegate interface {
	Write(v any) ([]byte, error)
}

// LiteralBytes are the byte forms of JSON literals.
var (
	nullBytes  = []byte(LiteralNull)
	trueBytes  = []byte(LiteralTrue)
	falseBytes = []byte(LiteralFalse)
)

// ValueWriter appends Go tree values exactly as codec.TreeWriter encodes them: sorted
// object keys, compact, canonical strings and numbers.
type ValueWriter struct {
	Delegate Delegate
	keys     [][]string
}

// Append writes v onto dst.
func (w *ValueWriter) Append(dst []byte, v any) ([]byte, error) {
	return w.value(dst, v, 0)
}

func (w *ValueWriter) value(dst []byte, v any, depth int) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return append(dst, nullBytes...), nil
	case bool:
		return w.boolean(dst, t), nil
	case string:
		return Strings{}.Append(dst, t), nil
	case int64:
		return strconv.AppendInt(dst, t, 10), nil
	case float64:
		if !math.IsNaN(t) && !math.IsInf(t, 0) {
			return jsontext.AppendFloat(dst, t, 64), nil
		}
	case map[string]any:
		return w.object(dst, t, depth)
	case []any:
		return w.array(dst, t, depth)
	}
	return w.delegate(dst, v)
}

func (ValueWriter) boolean(dst []byte, b bool) []byte {
	if b {
		return append(dst, trueBytes...)
	}
	return append(dst, falseBytes...)
}

func (w *ValueWriter) delegate(dst []byte, v any) ([]byte, error) {
	b, err := w.Delegate.Write(v)
	if err != nil {
		return dst, err
	}
	return append(dst, b...), nil
}

// sortedKeys returns the keys of m in order, reusing the slice of depth d.
func (w *ValueWriter) sortedKeys(m map[string]any, d int) []string {
	for len(w.keys) <= d {
		w.keys = append(w.keys, nil)
	}
	keys := w.keys[d][:0]
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	w.keys[d] = keys
	return keys
}

func (w *ValueWriter) object(dst []byte, m map[string]any, depth int) ([]byte, error) {
	keys := w.sortedKeys(m, depth)
	dst = append(dst, byte(ObjectOpen))
	var err error
	for n, k := range keys {
		if n > 0 {
			dst = append(dst, byte(Comma))
		}
		dst = append(Strings{}.Append(dst, k), byte(Colon))
		if dst, err = w.value(dst, m[k], depth+1); err != nil {
			return dst, err
		}
	}
	clear(keys)
	return append(dst, byte(ObjectClose)), nil
}

func (w *ValueWriter) array(dst []byte, a []any, depth int) ([]byte, error) {
	dst = append(dst, byte(ArrayOpen))
	var err error
	for n, el := range a {
		if n > 0 {
			dst = append(dst, byte(Comma))
		}
		if dst, err = w.value(dst, el, depth+1); err != nil {
			return dst, err
		}
	}
	return append(dst, byte(ArrayClose)), nil
}
