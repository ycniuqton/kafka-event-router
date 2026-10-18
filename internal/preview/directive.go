package preview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// DirectiveHeader is the internal header carrying the signed caller directive (API → preview).
type DirectiveHeader string

const HeaderCaller DirectiveHeader = "X-Router-Caller"

// DirectivePolicy bounds the validity of a directive.
type DirectivePolicy time.Duration

const DirectiveTTL DirectivePolicy = DirectivePolicy(time.Minute)

// DirectiveSyntax names the separator between payload and signature.
type DirectiveSyntax string

const DirectiveSeparator DirectiveSyntax = "."

// errDirective is the cause of every rejected directive (details are never shown).
var errDirective = errors.New("invalid caller directive")

// directiveEnvelope is the signed payload.
type directiveEnvelope struct {
	Caller    Caller `json:"caller"`
	ExpiresAt int64  `json:"exp"`
}

// DirectiveCodec signs and verifies caller directives with HMAC-SHA256 over a shared secret.
type DirectiveCodec struct {
	secret []byte
	now    func() time.Time
}

// NewDirectiveCodec builds a codec.
func NewDirectiveCodec(secret string, now func() time.Time) DirectiveCodec {
	return DirectiveCodec{secret: []byte(secret), now: now}
}

// Encode returns the header value for caller.
func (c DirectiveCodec) Encode(caller Caller) (string, error) {
	raw, err := json.Marshal(directiveEnvelope{Caller: caller, ExpiresAt: c.now().Add(time.Duration(DirectiveTTL)).Unix()})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + string(DirectiveSeparator) + c.sign(payload), nil
}

// Decode verifies the signature and expiry and returns the caller.
func (c DirectiveCodec) Decode(value string) (Caller, error) {
	payload, sig, ok := strings.Cut(value, string(DirectiveSeparator))
	if !ok || !hmac.Equal([]byte(sig), []byte(c.sign(payload))) {
		return Caller{}, errDirective
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Caller{}, errDirective
	}
	var env directiveEnvelope
	if json.Unmarshal(raw, &env) != nil || c.now().Unix() > env.ExpiresAt || env.Caller.UserID == "" {
		return Caller{}, errDirective
	}
	return env.Caller, nil
}

func (c DirectiveCodec) sign(payload string) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
