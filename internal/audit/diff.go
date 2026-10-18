package audit

import (
	"encoding/json"
	"reflect"

	"github.com/eventrouter/router/internal/platform/logging"
)

// DiffBuilder computes a JSON merge-patch (RFC 7396) from before to after with secret fields redacted.
type DiffBuilder struct {
	redactor *logging.Redactor
}

// NewDiffBuilder returns a builder using the platform redactor.
func NewDiffBuilder(redactor *logging.Redactor) *DiffBuilder {
	return &DiffBuilder{redactor: redactor}
}

// Build returns the merge-patch turning before into after; nil before means creation (full after).
func (b *DiffBuilder) Build(before, after any) (json.RawMessage, error) {
	from, err := b.toMap(before)
	if err != nil {
		return nil, err
	}
	to, err := b.toMap(after)
	if err != nil {
		return nil, err
	}
	return json.Marshal(b.patch(from, to))
}

func (b *DiffBuilder) toMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *DiffBuilder) patch(from, to map[string]any) map[string]any {
	out := map[string]any{}
	for key, newVal := range to {
		oldVal, existed := from[key]
		if existed && reflect.DeepEqual(oldVal, newVal) {
			continue
		}
		out[key] = b.changedValue(key, oldVal, newVal)
	}
	for key := range from {
		if _, kept := to[key]; !kept {
			out[key] = nil
		}
	}
	return out
}

func (b *DiffBuilder) changedValue(key string, oldVal, newVal any) any {
	if b.redactor.IsSensitive(key) {
		return string(logging.Redacted)
	}
	oldMap, oldIsMap := oldVal.(map[string]any)
	newMap, newIsMap := newVal.(map[string]any)
	if oldIsMap && newIsMap {
		return b.patch(oldMap, newMap)
	}
	if newIsMap {
		return b.patch(map[string]any{}, newMap)
	}
	return newVal
}
