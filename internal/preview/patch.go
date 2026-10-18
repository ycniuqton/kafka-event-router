package preview

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// PatchOpName names the RFC 6902 operations preview emits (add, remove, replace only).
type PatchOpName string

const (
	OpAdd     PatchOpName = "add"
	OpRemove  PatchOpName = "remove"
	OpReplace PatchOpName = "replace"
)

// PatchDiffer computes the RFC 6902 patch turning prev into next.
type PatchDiffer interface {
	Diff(prev, next any) []trace.PatchOp
}

// StructuralDiffer diffs objects member-wise and arrays index-wise; emits add/remove/replace only,
// so the TS JsonPatchApplier can replay patches without move/copy support.
type StructuralDiffer struct{}

// Diff implements PatchDiffer.
func (d StructuralDiffer) Diff(prev, next any) []trace.PatchOp {
	ops := []trace.PatchOp{}
	d.walk(config.PointerRoot, prev, next, &ops)
	return ops
}

func (d StructuralDiffer) walk(p config.Pointer, a, b any, ops *[]trace.PatchOp) {
	am, aObj := a.(map[string]any)
	bm, bObj := b.(map[string]any)
	if aObj && bObj {
		d.objects(p, am, bm, ops)
		return
	}
	al, aArr := a.([]any)
	bl, bArr := b.([]any)
	if aArr && bArr {
		d.arrays(p, al, bl, ops)
		return
	}
	if !(ValueEquality{}).Equal(a, b) {
		*ops = append(*ops, trace.PatchOp{Op: string(OpReplace), Path: string(p), Value: b, HasValue: true})
	}
}

func (d StructuralDiffer) objects(p config.Pointer, a, b map[string]any, ops *[]trace.PatchOp) {
	for _, k := range (JSONValues{}).SortedKeys(a) {
		if _, ok := b[k]; !ok {
			*ops = append(*ops, trace.PatchOp{Op: string(OpRemove), Path: string(p.Key(k))})
		}
	}
	for _, k := range (JSONValues{}).SortedKeys(b) {
		prev, ok := a[k]
		if !ok {
			*ops = append(*ops, trace.PatchOp{Op: string(OpAdd), Path: string(p.Key(k)), Value: b[k], HasValue: true})
			continue
		}
		d.walk(p.Key(k), prev, b[k], ops)
	}
}

func (d StructuralDiffer) arrays(p config.Pointer, a, b []any, ops *[]trace.PatchOp) {
	common := min(len(a), len(b))
	for i := 0; i < common; i++ {
		d.walk(p.Index(i), a[i], b[i], ops)
	}
	for i := common; i < len(b); i++ {
		*ops = append(*ops, trace.PatchOp{Op: string(OpAdd), Path: string(p.Index(i)), Value: b[i], HasValue: true})
	}
	for i := len(a) - 1; i >= len(b); i-- {
		*ops = append(*ops, trace.PatchOp{Op: string(OpRemove), Path: string(p.Index(i))})
	}
}

// ValueEquality compares decoded scalars; numbers compare by value across Go types.
type ValueEquality struct{}

// Equal reports whether a and b are the same JSON value.
func (e ValueEquality) Equal(a, b any) bool {
	fa, aNum := e.number(a)
	fb, bNum := e.number(b)
	if aNum && bNum {
		return fa == fb
	}
	return reflect.DeepEqual(a, b)
}

func (ValueEquality) number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// PatchApplier replays add/remove/replace patches on a deep copy of a document.
type PatchApplier struct{}

// Apply returns doc with ops applied; unknown ops and bad paths are errors.
func (a PatchApplier) Apply(doc any, ops []trace.PatchOp) (any, error) {
	out := engine.ValueCloner{}.Clone(doc)
	for _, op := range ops {
		next, err := a.at(out, config.Pointer(op.Path).Tokens(), op)
		if err != nil {
			return nil, fmt.Errorf("patch %s %s: %w", op.Op, op.Path, err)
		}
		out = next
	}
	return out, nil
}

// at applies op below node and returns the (possibly new) node.
func (a PatchApplier) at(node any, tokens []string, op trace.PatchOp) (any, error) {
	if len(tokens) == 0 {
		return engine.ValueCloner{}.Clone(op.Value), nil
	}
	switch n := node.(type) {
	case map[string]any:
		return a.inObject(n, tokens, op)
	case []any:
		return a.inArray(n, tokens, op)
	}
	return nil, fmt.Errorf("cannot descend into scalar at %q", tokens[0])
}

func (a PatchApplier) inObject(obj map[string]any, tokens []string, op trace.PatchOp) (any, error) {
	if len(tokens) == 1 {
		return obj, ObjectPatcher{}.Apply(obj, tokens[0], op)
	}
	child, ok := obj[tokens[0]]
	if !ok {
		return nil, fmt.Errorf("member %q missing", tokens[0])
	}
	v, err := a.at(child, tokens[1:], op)
	obj[tokens[0]] = v
	return obj, err
}

func (a PatchApplier) inArray(list []any, tokens []string, op trace.PatchOp) (any, error) {
	i, err := strconv.Atoi(tokens[0])
	if err != nil || i < 0 || i > len(list) {
		return nil, fmt.Errorf("index %q out of range", tokens[0])
	}
	if len(tokens) == 1 {
		return ArrayPatcher{}.Apply(list, i, op)
	}
	if i == len(list) {
		return nil, fmt.Errorf("index %q out of range", tokens[0])
	}
	v, err := a.at(list[i], tokens[1:], op)
	list[i] = v
	return list, err
}

// ObjectPatcher applies one op to an object member.
type ObjectPatcher struct{}

// Apply sets or deletes member key.
func (ObjectPatcher) Apply(obj map[string]any, key string, op trace.PatchOp) error {
	switch PatchOpName(op.Op) {
	case OpAdd, OpReplace:
		obj[key] = engine.ValueCloner{}.Clone(op.Value)
	case OpRemove:
		delete(obj, key)
	default:
		return fmt.Errorf("unsupported op")
	}
	return nil
}

// ArrayPatcher applies one op to an array element.
type ArrayPatcher struct{}

// Apply inserts, replaces or removes element i and returns the resulting array.
func (ArrayPatcher) Apply(list []any, i int, op trace.PatchOp) ([]any, error) {
	switch PatchOpName(op.Op) {
	case OpAdd:
		out := append([]any{}, list[:i]...)
		out = append(out, engine.ValueCloner{}.Clone(op.Value))
		return append(out, list[i:]...), nil
	case OpReplace, OpRemove:
		if i >= len(list) {
			return nil, fmt.Errorf("index %d out of range", i)
		}
	default:
		return nil, fmt.Errorf("unsupported op")
	}
	if PatchOpName(op.Op) == OpReplace {
		list[i] = engine.ValueCloner{}.Clone(op.Value)
		return list, nil
	}
	return append(list[:i:i], list[i+1:]...), nil
}
