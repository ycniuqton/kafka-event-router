package codec

import (
	"encoding/binary"
	"fmt"
)

// WireFormat holds the Confluent schema-registry framing constants.
type WireFormat byte

const (
	WireMagicByte  WireFormat = 0x00
	WireHeaderSize            = 5
)

// ConfluentFrame is a framed payload: magic byte, big-endian schema id, body.
type ConfluentFrame struct {
	SchemaID int32
	Body     []byte
}

// Framing reads and writes Confluent wire framing.
type Framing struct{}

// Parse splits raw into schema id and body; unframed bytes are malformed.
func (Framing) Parse(raw []byte) (ConfluentFrame, error) {
	if len(raw) < WireHeaderSize || raw[0] != byte(WireMagicByte) {
		return ConfluentFrame{}, fmt.Errorf("%w: not schema-registry framed", ErrMalformed)
	}
	return ConfluentFrame{SchemaID: int32(binary.BigEndian.Uint32(raw[1:WireHeaderSize])), Body: raw[WireHeaderSize:]}, nil
}

// Write prefixes body with the magic byte and schema id.
func (Framing) Write(id int32, body []byte) []byte {
	out := make([]byte, WireHeaderSize, WireHeaderSize+len(body))
	out[0] = byte(WireMagicByte)
	binary.BigEndian.PutUint32(out[1:WireHeaderSize], uint32(id))
	return append(out, body...)
}
