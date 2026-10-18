package preview

import (
	"strconv"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// EventPart is the part of a trace event document (engine.Event.Snapshot) a patch value stands for.
type EventPart int

const (
	PartPayload EventPart = iota
	PartList
	PartDocument
	PartHeaders
	PartHeader
	PartHeaderName
	PartMetadata
)

// EventPointer locates a step patch path in the trace document: one event, or a list of events when
// the step changed the event count (paths then start with the list index).
type EventPointer struct{}

// Part returns what the value at path is; the root is a list when its value is an array.
func (e EventPointer) Part(path string, value any) EventPart {
	tokens := config.Pointer(path).Tokens()
	if len(tokens) == 0 {
		if _, list := value.([]any); list {
			return PartList
		}
		return PartDocument
	}
	if e.isIndex(tokens[0]) {
		tokens = tokens[1:]
	}
	return e.member(tokens)
}

// member classifies tokens relative to one event document.
func (e EventPointer) member(tokens []string) EventPart {
	if len(tokens) == 0 {
		return PartDocument
	}
	switch engine.EventField(tokens[0]) {
	case engine.FieldHeaders:
		return e.header(tokens[1:])
	case engine.FieldTopic, engine.FieldPartition, engine.FieldOffset, engine.FieldTimestamp:
		return PartMetadata
	}
	return PartPayload
}

// header classifies tokens relative to the header list: the list, one entry, its name or its value.
func (EventPointer) header(tokens []string) EventPart {
	switch {
	case len(tokens) == 0:
		return PartHeaders
	case len(tokens) == 1:
		return PartHeader
	case len(tokens) == 2 && engine.EventField(tokens[1]) == engine.FieldKey:
		return PartHeaderName
	}
	return PartPayload
}

func (EventPointer) isIndex(token string) bool {
	_, err := strconv.Atoi(token)
	return err == nil
}

// PatchMasker masks step patch values like samples are masked: payload values (key, value, header values)
// are masked, header names and event metadata are kept.
type PatchMasker struct {
	masker ValueMasker
}

// Op returns op with its value masked according to the document part it replaces.
func (p PatchMasker) Op(op trace.PatchOp) trace.PatchOp {
	op.Value = p.part(EventPointer{}.Part(op.Path, op.Value), op.Value)
	return op
}

// part masks v as the given document part.
func (p PatchMasker) part(part EventPart, v any) any {
	switch part {
	case PartList:
		return p.list(v)
	case PartDocument:
		return p.document(v)
	case PartHeaders:
		return p.headers(v)
	case PartHeader:
		return p.header(v)
	case PartHeaderName, PartMetadata:
		return v
	}
	return p.masker.Mask(v)
}

// list masks a list of event documents.
func (p PatchMasker) list(v any) any {
	items, ok := v.([]any)
	if !ok {
		return p.masker.Mask(v)
	}
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = p.document(item)
	}
	return out
}

// document masks one event document member by member.
func (p PatchMasker) document(v any) any {
	doc, ok := v.(map[string]any)
	if !ok {
		return p.masker.Mask(v)
	}
	out := make(map[string]any, len(doc))
	for name, child := range doc {
		out[name] = p.part(EventPointer{}.member([]string{name}), child)
	}
	return out
}

// headers masks a header list.
func (p PatchMasker) headers(v any) any {
	items, ok := v.([]any)
	if !ok {
		return p.masker.Mask(v)
	}
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = p.header(item)
	}
	return out
}

// header masks one {key, value} header entry: the name is kept, everything else masked.
func (p PatchMasker) header(v any) any {
	entry, ok := v.(map[string]any)
	if !ok {
		return p.masker.Mask(v)
	}
	out := make(map[string]any, len(entry))
	for name, child := range entry {
		out[name] = child
		if engine.EventField(name) != engine.FieldKey {
			out[name] = p.masker.Mask(child)
		}
	}
	return out
}
