package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HashPrefix tags config hashes with their algorithm.
type HashPrefix string

const HashSHA256 HashPrefix = "sha256:"

// ContentHasher fingerprints a version's content independent of key order and whitespace.
type ContentHasher struct{}

// Hash returns "sha256:<hex>" of the canonical JSON of content.
func (ContentHasher) Hash(content json.RawMessage) (string, error) {
	var tree any
	if err := json.Unmarshal(content, &tree); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(tree)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return string(HashSHA256) + hex.EncodeToString(sum[:]), nil
}
