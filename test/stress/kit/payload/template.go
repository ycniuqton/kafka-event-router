// Package payload renders stress events: a JSON template padded to a target size whose seq/stream/bad members are
// filled per event without re-encoding the document.
package payload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Field names the producer owns inside every event.
type Field string

const (
	FieldSeq    Field = "seq"
	FieldStream Field = "stream"
	FieldBad    Field = "bad"
	FieldPad    Field = "pad"
)

// Slot is a per-event value spliced into the rendered template.
type Slot int

const (
	SlotSeq Slot = iota
	SlotStream
	SlotBad
)

// Placeholders are the sentinel strings marking slots in the encoded template.
var Placeholders = map[Slot]string{SlotSeq: `"__stress_seq__"`, SlotStream: `"__stress_stream__"`, SlotBad: `"__stress_bad__"`}

// Sizing holds the padding assumptions: digits reserved for seq and the pad filler byte.
var Sizing = struct {
	SeqDigits int
	Filler    string
}{SeqDigits: 9, Filler: "x"}

// DefaultTemplate is an order-like event used when no template file is given.
const DefaultTemplate = `{"type":"order","eventType":"OrderPaid","orderId":"o-1","customer":{"id":"c-1","email":"jane@example.com","country":"VN"},"amount":42.5,"currency":"USD","items":[{"sku":"sku-1","qty":2,"price":10.25},{"sku":"sku-2","qty":1,"price":22}]}`

// Template is a pre-split event document.
type Template struct {
	parts [][]byte
	slots []Slot
	size  int
}

// splitter cuts an encoded template at its slot placeholders.
type splitter struct{}

// Event is the per-event values filled into the template.
type Event struct {
	Seq    int64
	Stream string
	Bad    bool
}

// Compile parses a JSON object template and pads it (member "pad") so rendered events are about size bytes.
func Compile(doc []byte, size int) (*Template, error) {
	obj := map[string]any{}
	if err := json.Unmarshal(doc, &obj); err != nil {
		return nil, fmt.Errorf("payload template must be a JSON object: %w", err)
	}
	obj[string(FieldSeq)] = strings.Trim(Placeholders[SlotSeq], `"`)
	obj[string(FieldStream)] = strings.Trim(Placeholders[SlotStream], `"`)
	obj[string(FieldBad)] = strings.Trim(Placeholders[SlotBad], `"`)
	t, err := splitter{}.split(obj)
	if err != nil {
		return nil, err
	}
	if pad := size - t.estimate(); pad > 0 {
		obj[string(FieldPad)] = strings.Repeat(Sizing.Filler, pad)
		return splitter{}.split(obj)
	}
	return t, nil
}

// Append renders one event onto dst.
func (t *Template) Append(dst []byte, e Event) []byte {
	for i, part := range t.parts {
		dst = append(dst, part...)
		if i < len(t.slots) {
			dst = t.fill(dst, t.slots[i], e)
		}
	}
	return dst
}

// EstimatedSize is the rendered size of a typical event.
func (t *Template) EstimatedSize() int {
	return t.estimate()
}

func (t *Template) fill(dst []byte, s Slot, e Event) []byte {
	switch s {
	case SlotSeq:
		return strconv.AppendInt(dst, e.Seq, 10)
	case SlotStream:
		return strconv.AppendQuote(dst, e.Stream)
	default:
		return strconv.AppendBool(dst, e.Bad)
	}
}

func (t *Template) estimate() int {
	return t.size + Sizing.SeqDigits + len(`"s-0000"`) + len("false")
}

func (splitter) split(obj map[string]any) (*Template, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	t := &Template{}
	for len(raw) > 0 {
		slot, at := splitter{}.nextSlot(raw)
		if at < 0 {
			t.parts = append(t.parts, raw)
			break
		}
		t.parts = append(t.parts, raw[:at])
		t.slots = append(t.slots, slot)
		raw = raw[at+len(Placeholders[slot]):]
	}
	for _, p := range t.parts {
		t.size += len(p)
	}
	return t, nil
}

func (splitter) nextSlot(raw []byte) (Slot, int) {
	best, at := SlotSeq, -1
	for slot, ph := range Placeholders {
		if i := bytes.Index(raw, []byte(ph)); i >= 0 && (at < 0 || i < at) {
			best, at = slot, i
		}
	}
	return best, at
}
