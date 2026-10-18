package path

import (
	"errors"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/engine"
)

// ErrReadOnly reports a write to event metadata.
var ErrReadOnly = errors.New("field is read-only")

// ErrNotContainer reports a write through a scalar or a missing array.
var ErrNotContainer = errors.New("cannot write through a non-container value")

// IsRoot reports whether the path addresses a whole root member (no segments).
func (p FieldPath) IsRoot() bool { return len(p.Segments) == 0 }

// Get returns the value at the path and whether it exists.
func (p FieldPath) Get(ev *engine.Event) (any, bool) {
	switch p.Root {
	case RootValue:
		return tree{}.get(ev.Value, p.Segments)
	case RootKey:
		return tree{}.get(ev.Key, p.Segments)
	case RootHeaders:
		return headerList{}.get(ev.Headers, p.Segments[0].Name)
	}
	return metadata{}.get(ev, p.Root)
}

// Exists reports whether the path resolves.
func (p FieldPath) Exists(ev *engine.Event) bool {
	_, ok := p.Get(ev)
	return ok
}

// Set writes v, creating intermediate objects; metadata roots are read-only.
func (p FieldPath) Set(ev *engine.Event, v any) error {
	var err error
	switch p.Root {
	case RootValue:
		ev.Value, err = tree{}.set(ev.Value, p.Segments, v)
	case RootKey:
		ev.Key, err = tree{}.set(ev.Key, p.Segments, v)
	case RootHeaders:
		ev.Headers = headerList{}.set(ev.Headers, p.Segments[0].Name, v)
	default:
		err = fmt.Errorf("%w: %s", ErrReadOnly, p.raw)
	}
	return err
}

// Delete removes the path and reports whether something was removed.
func (p FieldPath) Delete(ev *engine.Event) bool {
	var removed bool
	switch p.Root {
	case RootValue:
		ev.Value, removed = tree{}.del(ev.Value, p.Segments)
	case RootKey:
		ev.Key, removed = tree{}.del(ev.Key, p.Segments)
	case RootHeaders:
		ev.Headers, removed = headerList{}.del(ev.Headers, p.Segments[0].Name)
	}
	return removed
}

// tree navigates generic decoded values.
type tree struct{}

func (t tree) get(node any, segs []Segment) (any, bool) {
	cur := node
	for _, s := range segs {
		next, ok := t.child(cur, s)
		if !ok {
			return nil, false
		}
		cur = next
	}
	if len(segs) == 0 && node == nil {
		return nil, false
	}
	return cur, true
}

func (tree) child(node any, s Segment) (any, bool) {
	if s.IsIndex {
		arr, ok := node.([]any)
		if !ok || s.Index >= len(arr) {
			return nil, false
		}
		return arr[s.Index], true
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[s.Name]
	return v, ok
}

func (t tree) set(node any, segs []Segment, v any) (any, error) {
	if len(segs) == 0 {
		return v, nil
	}
	s := segs[0]
	if s.IsIndex {
		arr, ok := node.([]any)
		if !ok || s.Index >= len(arr) {
			return node, ErrNotContainer
		}
		child, err := t.set(arr[s.Index], segs[1:], v)
		arr[s.Index] = child
		return arr, err
	}
	m, ok := node.(map[string]any)
	if node == nil {
		m, ok = map[string]any{}, true
	}
	if !ok {
		return node, ErrNotContainer
	}
	child, err := t.set(m[s.Name], segs[1:], v)
	if err == nil {
		m[s.Name] = child
	}
	return m, err
}

func (t tree) del(node any, segs []Segment) (any, bool) {
	if len(segs) == 0 {
		return nil, node != nil
	}
	if len(segs) == 1 {
		return t.removeChild(node, segs[0])
	}
	child, ok := t.child(node, segs[0])
	if !ok {
		return node, false
	}
	newChild, removed := t.del(child, segs[1:])
	t.replaceChild(node, segs[0], newChild)
	return node, removed
}

func (tree) replaceChild(node any, s Segment, v any) {
	if arr, ok := node.([]any); ok && s.IsIndex {
		arr[s.Index] = v
		return
	}
	if m, ok := node.(map[string]any); ok && !s.IsIndex {
		m[s.Name] = v
	}
}

func (tree) removeChild(parent any, s Segment) (any, bool) {
	if s.IsIndex {
		arr, ok := parent.([]any)
		if !ok || s.Index >= len(arr) {
			return parent, false
		}
		out := append(append([]any{}, arr[:s.Index]...), arr[s.Index+1:]...)
		return out, true
	}
	m, ok := parent.(map[string]any)
	if !ok {
		return parent, false
	}
	_, had := m[s.Name]
	delete(m, s.Name)
	return m, had
}

// headerList reads and writes headers by name (first value wins on read).
type headerList struct{}

func (headerList) get(hs []engine.Header, name string) (any, bool) {
	for _, h := range hs {
		if h.Key == name {
			return string(h.Value), true
		}
	}
	return nil, false
}

func (h headerList) set(hs []engine.Header, name string, v any) []engine.Header {
	out, _ := h.del(hs, name)
	return append(out, engine.Header{Key: name, Value: []byte(headerText{}.of(v))})
}

func (headerList) del(hs []engine.Header, name string) ([]engine.Header, bool) {
	out := hs[:0:0]
	removed := false
	for _, h := range hs {
		if h.Key == name {
			removed = true
			continue
		}
		out = append(out, h)
	}
	return out, removed
}

// headerText renders header values as text.
type headerText struct{}

func (headerText) of(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// metadata exposes read-only record metadata.
type metadata struct{}

func (metadata) get(ev *engine.Event, root PathRoot) (any, bool) {
	switch root {
	case RootTopic:
		return ev.Topic, true
	case RootPartition:
		return int64(ev.Partition), true
	case RootOffset:
		return ev.Offset, true
	case RootTimestamp:
		return ev.Timestamp.UTC().Format(time.RFC3339Nano), true
	}
	return nil, false
}
