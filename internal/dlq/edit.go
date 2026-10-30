package dlq

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// EditLimit bounds edited records (design 00001 §4.1).
type EditLimit int

const MaxEditBytes EditLimit = 1 << 20

// RecordEdit replaces parts of a dead letter before replay; a nil member keeps the original.
type RecordEdit struct {
	Key     *EncodedValue    `json:"key,omitempty"`
	Value   *EncodedValue    `json:"value,omitempty"`
	Headers *[]EncodedHeader `json:"headers,omitempty"`
}

// Empty reports whether the edit changes nothing.
func (e *RecordEdit) Empty() bool {
	return e == nil || (e.Key == nil && e.Value == nil && e.Headers == nil)
}

// DiffOp is one structural change.
type DiffOp string

const (
	DiffAdd     DiffOp = "add"
	DiffRemove  DiffOp = "remove"
	DiffReplace DiffOp = "replace"
)

// DiffEntry is one changed JSON path (values are never kept: audit stores paths only).
type DiffEntry struct {
	Path string `json:"path"`
	Op   DiffOp `json:"op"`
}

// EditPlan is a record ready for replay: the edited bytes, the change paths and whether masks were merged.
type EditPlan struct {
	Record record.Record
	Diff   []DiffEntry
	Masked bool
	Edited bool
}

// EditBytes converts edited encoded values back into bytes.
type EditBytes struct{}

// Of returns the bytes of v; a JSON null is a nil (absent) key or value.
func (EditBytes) Of(v EncodedValue, path RequestPath) ([]byte, error) {
	switch v.Encoding {
	case EncodingJSON:
		if v.Data == nil {
			return nil, nil
		}
		return json.Marshal(v.Data)
	case EncodingUTF8:
		s, ok := v.Data.(string)
		if !ok {
			return nil, ErrInvalidEdit.At(string(path))
		}
		return []byte(s), nil
	case EncodingBase64:
		s, _ := v.Data.(string)
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, ErrInvalidEdit.At(string(path))
		}
		return b, nil
	case EncodingAvro, EncodingProtobuf:
		return nil, ErrEditUnsupported.At(string(path))
	}
	return nil, ErrInvalidEdit.At(string(path))
}

// SchemaBackedFormats are source formats whose payloads cannot be edited (no encoder for edited JSON).
var SchemaBackedFormats = map[config.FormatType]bool{config.FormatTypeAvro: true, config.FormatTypeProtobuf: true}

// EditFormatCheck validates edited bytes against the source format of the replaying version.
type EditFormatCheck struct{}

// Check rejects edits of schema-backed payloads and non-JSON bytes for JSON formats.
func (EditFormatCheck) Check(format config.Format, b []byte, path RequestPath) error {
	if SchemaBackedFormats[format.Type] {
		return ErrEditUnsupported.At(string(path))
	}
	if format.Type == config.FormatTypeJSON && b != nil && !json.Valid(b) {
		return ErrInvalidEdit.At(string(path))
	}
	return nil
}

// JSONTree decodes bytes into a comparable tree: JSON when valid, else the text itself.
type JSONTree struct{}

// Of returns the tree of b (nil for nil bytes).
func (JSONTree) Of(b []byte) any {
	if b == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) == nil && !dec.More() {
		return v
	}
	return string(b)
}

// Same reports whether two trees serialize identically.
func (JSONTree) Same(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// MaskMerger restores masked leaves: a leaf the editor left at the mask of the original is the original
// (masked editors never receive raw values, so untouched masked fields must not be replayed masked).
type MaskMerger struct {
	Masker ValueMasker
}

// Tree merges edited over original.
func (m MaskMerger) Tree(edited, original any) any {
	switch e := edited.(type) {
	case map[string]any:
		o, ok := original.(map[string]any)
		if !ok {
			return edited
		}
		out := make(map[string]any, len(e))
		for k, v := range e {
			out[k] = m.child(v, o, k)
		}
		return out
	case []any:
		o, ok := original.([]any)
		if !ok {
			return edited
		}
		out := make([]any, len(e))
		for i, v := range e {
			out[i] = m.index(v, o, i)
		}
		return out
	}
	return m.leaf(edited, original)
}

func (m MaskMerger) child(v any, o map[string]any, k string) any {
	if ov, ok := o[k]; ok {
		return m.Tree(v, ov)
	}
	return v
}

func (m MaskMerger) index(v any, o []any, i int) any {
	if i < len(o) {
		return m.Tree(v, o[i])
	}
	return v
}

func (m MaskMerger) leaf(edited, original any) any {
	if original != nil && (JSONTree{}).Same(edited, m.Masker.Mask(original)) {
		return original
	}
	return edited
}

// Text merges a masked text value (a utf8 or base64 payload shown as masked text).
func (m MaskMerger) Text(edited []byte, original []byte) []byte {
	if bytes.Equal(edited, []byte(PayloadMasker{Masker: m.Masker}.Text(string(original)))) {
		return original
	}
	return edited
}

// Bytes merges edited payload bytes over the original ones.
func (m MaskMerger) Bytes(edited, original []byte) []byte {
	if edited == nil || original == nil {
		return edited
	}
	e, o := JSONTree{}.Of(edited), JSONTree{}.Of(original)
	if _, isText := o.(string); isText {
		return m.Text(edited, original)
	}
	out, err := json.Marshal(m.Tree(e, o))
	if err != nil {
		return edited
	}
	return out
}

// StructuralDiff lists the changed JSON paths between two trees (RFC 6901 pointers).
type StructuralDiff struct{}

// Of appends the changes below prefix.
func (d StructuralDiff) Of(prefix string, before, after any) []DiffEntry {
	bm, bIsMap := before.(map[string]any)
	am, aIsMap := after.(map[string]any)
	if bIsMap && aIsMap {
		return d.maps(prefix, bm, am)
	}
	ba, bIsArr := before.([]any)
	aa, aIsArr := after.([]any)
	if bIsArr && aIsArr {
		return d.arrays(prefix, ba, aa)
	}
	if (JSONTree{}).Same(before, after) {
		return nil
	}
	return []DiffEntry{{Path: prefix, Op: (DiffOps{}).Of(before, after)}}
}

func (d StructuralDiff) maps(prefix string, before, after map[string]any) []DiffEntry {
	out := []DiffEntry{}
	for _, k := range (SortedKeys{}).Of(before, after) {
		bv, inB := before[k]
		av, inA := after[k]
		path := prefix + "/" + (Pointer{}).Escape(k)
		switch {
		case !inA:
			out = append(out, DiffEntry{Path: path, Op: DiffRemove})
		case !inB:
			out = append(out, DiffEntry{Path: path, Op: DiffAdd})
		default:
			out = append(out, d.Of(path, bv, av)...)
		}
	}
	return out
}

func (d StructuralDiff) arrays(prefix string, before, after []any) []DiffEntry {
	out := []DiffEntry{}
	for i := 0; i < max(len(before), len(after)); i++ {
		path := prefix + "/" + strconv.Itoa(i)
		switch {
		case i >= len(after):
			out = append(out, DiffEntry{Path: path, Op: DiffRemove})
		case i >= len(before):
			out = append(out, DiffEntry{Path: path, Op: DiffAdd})
		default:
			out = append(out, d.Of(path, before[i], after[i])...)
		}
	}
	return out
}

// DiffOps picks the op of a changed leaf.
type DiffOps struct{}

// Of returns add/remove for nil transitions, else replace.
func (DiffOps) Of(before, after any) DiffOp {
	switch {
	case before == nil:
		return DiffAdd
	case after == nil:
		return DiffRemove
	}
	return DiffReplace
}

// SortedKeys returns the union of map keys, sorted.
type SortedKeys struct{}

// Of implements the union.
func (SortedKeys) Of(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Pointer escapes JSON Pointer tokens.
type Pointer struct{}

// Escape implements RFC 6901 (~ → ~0, / → ~1).
func (Pointer) Escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// HeaderTree turns headers into a diffable map (last value of a key wins).
type HeaderTree struct{}

// Of returns key → value.
func (HeaderTree) Of(hs []engine.Header) map[string]any {
	out := map[string]any{}
	for _, h := range hs {
		out[h.Key] = string(h.Value)
	}
	return out
}
