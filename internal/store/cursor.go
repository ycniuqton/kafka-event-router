package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Cursor is the keyset position after the last returned row: sort key values plus the row id.
type Cursor struct {
	SortKey []string `json:"k"`
	ID      string   `json:"i"`
}

// CursorPart separates the payload from its signature in the opaque token.
type CursorPart string

const CursorSeparator CursorPart = "."

// ErrInvalidCursor is returned for malformed or tampered cursors (400 at path /cursor).
var ErrInvalidCursor = &Error{
	Kind:   problem.KindMalformedBody,
	Detail: "The page cursor is invalid or expired. Restart from the first page.",
	Path:   "/cursor",
}

// CursorCodec encodes cursors as base64url(JSON) signed with HMAC-SHA256 to reject tampering.
type CursorCodec struct {
	key []byte
}

// NewCursorCodec builds a codec with the configured secret.
func NewCursorCodec(secret string) *CursorCodec {
	return &CursorCodec{key: []byte(secret)}
}

// Encode returns the opaque, signed token for c.
func (c *CursorCodec) Encode(cur Cursor) string {
	raw, _ := json.Marshal(cur)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + string(CursorSeparator) + base64.RawURLEncoding.EncodeToString(c.sign(payload))
}

// Decode verifies the signature and returns the cursor; any defect yields ErrInvalidCursor.
func (c *CursorCodec) Decode(token string) (Cursor, error) {
	payload, sig, ok := strings.Cut(token, string(CursorSeparator))
	if !ok {
		return Cursor{}, ErrInvalidCursor
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, c.sign(payload)) {
		return Cursor{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var cur Cursor
	if err := json.Unmarshal(raw, &cur); err != nil || cur.ID == "" {
		return Cursor{}, ErrInvalidCursor
	}
	return cur, nil
}

func (c *CursorCodec) sign(payload string) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
