package dlq

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Ref identifies a dead letter by its DLQ topic, partition and offset.
type Ref = record.Ref

// RefSeparator joins the parts of an encoded ref.
type RefSeparator string

const refSeparator RefSeparator = "|"

// NumberBase is the base of numbers inside ids and cursors.
type NumberBase int

const (
	decimal NumberBase = 10
	bits32  NumberBase = 32
	bits64  NumberBase = 64
)

// RefCodec converts refs to opaque URL-safe ids: base64url("topic|partition|offset").
type RefCodec struct{}

// Encode returns the opaque id of r.
func (RefCodec) Encode(r Ref) string {
	raw := strings.Join([]string{r.Topic, strconv.FormatInt(int64(r.Partition), int(decimal)), strconv.FormatInt(r.Offset, int(decimal))}, string(refSeparator))
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses an id; malformed ids return ErrInvalidID.
func (RefCodec) Decode(id string) (Ref, error) {
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return Ref{}, ErrInvalidID.Wrap(err)
	}
	s := string(raw)
	cut := strings.LastIndex(s, string(refSeparator))
	if cut < 0 {
		return Ref{}, ErrInvalidID
	}
	head := strings.LastIndex(s[:cut], string(refSeparator))
	if head <= 0 {
		return Ref{}, ErrInvalidID
	}
	p, perr := strconv.ParseInt(s[head+1:cut], int(decimal), int(bits32))
	o, oerr := strconv.ParseInt(s[cut+1:], int(decimal), int(bits64))
	if perr != nil || oerr != nil || p < 0 || o < 0 {
		return Ref{}, ErrInvalidID
	}
	return Ref{Topic: s[:head], Partition: int32(p), Offset: o}, nil
}

// DecodeAll parses ids; the first malformed one fails with its /ids/i path.
func (c RefCodec) DecodeAll(ids []string) ([]Ref, error) {
	out := make([]Ref, 0, len(ids))
	for i, id := range ids {
		r, err := c.Decode(id)
		if err != nil {
			return nil, ErrInvalidID.At(PathIDs.Indexed(i))
		}
		out = append(out, r)
	}
	return out, nil
}
