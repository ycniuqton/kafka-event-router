package trace

import "github.com/wI2L/jsondiff"

// PatchOpName names RFC 6902 operations that carry a value.
type PatchOpName string

const (
	PatchAdd     PatchOpName = "add"
	PatchReplace PatchOpName = "replace"
	PatchTest    PatchOpName = "test"
)

// PatchBuilder diffs step snapshots: one event in and out diffs the events directly,
// otherwise the event lists are diffed (paths start with the list index).
type PatchBuilder struct{}

// Diff returns the patch turning before into after; diff failures yield an empty patch.
func (PatchBuilder) Diff(before, after []any) []PatchOp {
	var src, dst any = before, after
	if len(before) == 1 && len(after) == 1 {
		src, dst = before[0], after[0]
	}
	patch, err := jsondiff.Compare(src, dst, jsondiff.Factorize())
	if err != nil {
		return []PatchOp{}
	}
	out := make([]PatchOp, 0, len(patch))
	for _, op := range patch {
		out = append(out, PatchOp{Op: op.Type, From: op.From, Path: op.Path, Value: op.Value, HasValue: PatchBuilder{}.carriesValue(op.Type)})
	}
	return out
}

func (PatchBuilder) carriesValue(op string) bool {
	switch PatchOpName(op) {
	case PatchAdd, PatchReplace, PatchTest:
		return true
	}
	return false
}
