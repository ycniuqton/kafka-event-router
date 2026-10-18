package dlq

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
)

// CursorVersion is the format version of encoded cursors; older versions are rejected.
type CursorVersion int

const CurrentCursorVersion CursorVersion = 1

// PartitionKey identifies one DLQ partition ("topic|partition").
type PartitionKey string

// KeyOf returns the partition key of a topic/partition.
func KeyOf(topic string, partition int32) PartitionKey {
	return PartitionKey(topic + string(refSeparator) + strconv.FormatInt(int64(partition), int(decimal)))
}

// Cursor holds, per partition, the exclusive upper offset the next page continues below.
type Cursor struct {
	V      CursorVersion          `json:"v"`
	Bounds map[PartitionKey]int64 `json:"b"`
	Done   bool                   `json:"d,omitempty"`
}

// NewCursor returns an empty current-version cursor.
func NewCursor() Cursor {
	return Cursor{V: CurrentCursorVersion, Bounds: map[PartitionKey]int64{}}
}

// Bound returns the upper bound of a partition, if the cursor has one.
func (c Cursor) Bound(k PartitionKey) (int64, bool) {
	v, ok := c.Bounds[k]
	return v, ok
}

// GlobalCursor is the composite cursor of the global list: one route cursor per route.
type GlobalCursor struct {
	V      CursorVersion     `json:"v"`
	Routes map[string]Cursor `json:"r"`
}

// CursorCodec converts cursors to opaque base64url JSON strings.
type CursorCodec struct{}

// Encode returns the opaque form of c.
func (CursorCodec) Encode(c any) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Decode parses a route cursor; empty means the first page.
func (cc CursorCodec) Decode(s string) (Cursor, error) {
	c := NewCursor()
	if s == "" {
		return c, nil
	}
	if err := cc.decode(s, &c); err != nil || c.V != CurrentCursorVersion || c.Bounds == nil {
		return Cursor{}, ErrInvalidCursor.At(string(PathCursor))
	}
	return c, nil
}

// DecodeGlobal parses a global cursor; empty means the first page.
func (cc CursorCodec) DecodeGlobal(s string) (GlobalCursor, error) {
	g := GlobalCursor{V: CurrentCursorVersion, Routes: map[string]Cursor{}}
	if s == "" {
		return g, nil
	}
	if err := cc.decode(s, &g); err != nil || g.V != CurrentCursorVersion || g.Routes == nil {
		return GlobalCursor{}, ErrInvalidCursor.At(string(PathCursor))
	}
	return g, nil
}

func (CursorCodec) decode(s string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
