// Package fastjson scans JSON bytes into a flat tape without building a value tree, and
// writes values back in exactly the form of codec.TreeWriter (sorted keys, compact,
// canonical strings and numbers), so callers can decode only the fields they need.
package fastjson

import (
	"sync"
	"unsafe"
)

// Kind is the JSON type of one tape entry.
type Kind uint8

const (
	KindNull Kind = iota + 1
	KindFalse
	KindTrue
	KindString
	KindNumber
	KindObject
	KindArray
)

// Flags describe how an entry's raw bytes relate to its canonical encoding.
type Flags uint8

const (
	// FlagCanonical marks raw bytes equal to the tree writer's output for the value.
	FlagCanonical Flags = 1 << iota
	// FlagEscaped marks strings that need unquoting (escapes or invalid UTF-8).
	FlagEscaped
	// FlagSorted marks objects whose unescaped member names strictly ascend.
	FlagSorted
)

// Entry is one value on the tape; an object stores key and value entries after it,
// an array its elements, and Next is the index just past the subtree.
type Entry struct {
	Start int32
	End   int32
	Next  int32
	Kind  Kind
	Flags Flags
}

// Canonical reports whether the raw bytes are already in tree-writer form.
func (e Entry) Canonical() bool { return e.Flags&FlagCanonical != 0 }

// Sorted reports whether an object's members are already in output order.
func (e Entry) Sorted() bool { return e.Flags&FlagSorted != 0 }

// Escaped reports whether a string entry needs unquoting.
func (e Entry) Escaped() bool { return e.Flags&FlagEscaped != 0 }

// Root is the tape index of the top-level value.
const Root int32 = 0

// Tape is the scanned form of one JSON document; Raw must not change while it is used.
type Tape struct {
	Raw       []byte
	Entries   []Entry
	Canonical bool
}

// At returns entry i.
func (t *Tape) At(i int32) Entry { return t.Entries[i] }

// Span returns the raw bytes of entry i.
func (t *Tape) Span(i int32) []byte {
	e := t.Entries[i]
	return t.Raw[e.Start:e.End]
}

// KeyView returns the decoded text of string entry i; unescaped strings share Raw's
// memory, so the result is valid only while the tape is.
func (t *Tape) KeyView(i int32) string {
	e := t.Entries[i]
	if e.Escaped() {
		return Strings{}.Decode(t.Raw[e.Start:e.End])
	}
	body := t.Raw[e.Start+1 : e.End-1]
	if len(body) == 0 {
		return ""
	}
	return unsafe.String(&body[0], len(body))
}

// Members iterates the key/value entry pairs of object entry obj.
type Members struct {
	t    *Tape
	next int32
	end  int32
}

// MembersOf starts an iteration over obj.
func (t *Tape) MembersOf(obj int32) Members {
	return Members{t: t, next: obj + 1, end: t.Entries[obj].Next}
}

// Next returns the next key and value indexes, ok false at the end.
func (m *Members) Next() (key, val int32, ok bool) {
	if m.next >= m.end {
		return 0, 0, false
	}
	key, val = m.next, m.next+1
	m.next = m.t.Entries[val].Next
	return key, val, true
}

// Elements iterates the element entries of array entry arr.
type Elements struct {
	t    *Tape
	next int32
	end  int32
}

// ElementsOf starts an iteration over arr.
func (t *Tape) ElementsOf(arr int32) Elements {
	return Elements{t: t, next: arr + 1, end: t.Entries[arr].Next}
}

// Next returns the next element index, ok false at the end.
func (e *Elements) Next() (int32, bool) {
	if e.next >= e.end {
		return 0, false
	}
	i := e.next
	e.next = e.t.Entries[i].Next
	return i, true
}

// TapePool recycles tapes between records.
type TapePool struct {
	pool sync.Pool
}

// Get returns an empty tape.
func (p *TapePool) Get() *Tape {
	if t, ok := p.pool.Get().(*Tape); ok {
		return t
	}
	return &Tape{}
}

// Put releases t; it must not be used afterwards.
func (p *TapePool) Put(t *Tape) {
	t.Raw, t.Entries, t.Canonical = nil, t.Entries[:0], false
	p.pool.Put(t)
}
