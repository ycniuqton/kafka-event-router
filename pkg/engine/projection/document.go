package projection

import (
	"bytes"
	"math"
	"sync"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/fastjson"
)

// ValueState says whether a branch can have changed the value it encodes.
type ValueState uint8

const (
	// ValueEdited values went through steps and are patched member by member.
	ValueEdited ValueState = iota
	// ValueUntouched values only went through filters and are written as decoded.
	ValueUntouched
)

// OutputSlack is the extra capacity reserved over the input size for patched output.
const OutputSlack = 64

// currentPos orders current members after every original member of the same name.
const currentPos int32 = math.MaxInt32

// Codec decodes record values into projections of one field tree and encodes them back.
type Codec struct {
	fields FieldTree
	docs   sync.Pool
}

// NewCodec builds the codec of a planned field tree.
func NewCodec(fields FieldTree) *Codec {
	return &Codec{fields: fields}
}

// Document is one decoded record value: its tape plus the writers that encode it.
type Document struct {
	codec   *Codec
	tape    fastjson.Tape
	scanner fastjson.Scanner
	canon   fastjson.CanonicalWriter
	values  fastjson.ValueWriter
	stack   fastjson.MemberStack
}

// Decode scans raw and returns the projection of an object value; ok is false when raw
// is not a JSON object the scanner accepts, and the caller uses the tree codec instead.
func (c *Codec) Decode(raw []byte) (*Document, map[string]any, bool) {
	d := c.document()
	if raw == nil || !d.scanner.Scan(raw, &d.tape) || d.tape.At(fastjson.Root).Kind != fastjson.KindObject {
		d.Release()
		return nil, nil, false
	}
	d.canon.Reset(&d.tape)
	return d, d.project(fastjson.Root, c.fields.Root), true
}

func (c *Codec) document() *Document {
	if d, ok := c.docs.Get().(*Document); ok {
		return d
	}
	return &Document{codec: c, values: fastjson.ValueWriter{Delegate: codec.TreeWriter{}}}
}

// Release returns the document to its codec; encoded outputs stay valid.
func (d *Document) Release() {
	d.tape.Raw, d.tape.Entries, d.tape.Canonical = nil, d.tape.Entries[:0], false
	d.codec.docs.Put(d)
}

// project decodes the members of object obj named in n (last duplicate wins).
func (d *Document) project(obj int32, n *FieldNode) map[string]any {
	out := map[string]any{}
	m := fastjson.Materializer{T: &d.tape}
	it := d.tape.MembersOf(obj)
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		child := n.Child(d.tape.KeyView(k))
		if child == nil {
			continue
		}
		if child.Partial() && d.tape.At(v).Kind == fastjson.KindObject {
			out[child.Name] = d.project(v, child)
			continue
		}
		out[child.Name] = m.Value(v)
	}
	return out
}

// Encode writes the value v of one output; untouched values are written straight from
// the tape and outputs equal to the input return the input bytes.
func (d *Document) Encode(v any, state ValueState) ([]byte, error) {
	root, ok := v.(map[string]any)
	if !ok {
		return d.values.Append(nil, v)
	}
	if state == ValueUntouched {
		if d.tape.Canonical {
			return d.tape.Raw, nil
		}
		return d.canon.Append(d.buffer(), fastjson.Root), nil
	}
	out, err := d.patch(d.buffer(), fastjson.Root, d.codec.fields.Root, root, 0)
	if err != nil {
		return nil, err
	}
	if d.tape.Canonical && bytes.Equal(out, d.tape.Raw) {
		return d.tape.Raw, nil
	}
	return out, nil
}

func (d *Document) buffer() []byte {
	return make([]byte, 0, len(d.tape.Raw)+OutputSlack)
}
