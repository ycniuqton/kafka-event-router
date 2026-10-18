package kafka

import (
	"encoding/json"
	"fmt"
)

// SecretCodec serialises a SecretBundle for envelope encryption (JSON bytes).
type SecretCodec struct{}

// Encode returns the bundle's JSON bytes.
func (SecretCodec) Encode(b SecretBundle) ([]byte, error) {
	return json.Marshal(b)
}

// Decode parses bundle bytes; empty input yields an empty bundle.
func (SecretCodec) Decode(raw []byte) (SecretBundle, error) {
	var b SecretBundle
	if len(raw) == 0 {
		return b, nil
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return SecretBundle{}, fmt.Errorf("kafka: decode secret bundle: %w", err)
	}
	return b, nil
}
