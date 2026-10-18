package oidc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// LoginState is what the encrypted er_oidc_state cookie carries between Begin and Complete.
type LoginState struct {
	State     string    `json:"s"`
	Nonce     string    `json:"n"`
	Verifier  string    `json:"v"`
	ReturnTo  string    `json:"r"`
	ExpiresAt time.Time `json:"e"`
}

// StateTTL is how long a started login may take.
type StateTTL time.Duration

const DefaultStateTTL StateTTL = StateTTL(10 * time.Minute)

var errBadState = errors.New("oidc: state cookie invalid")

// StateSealer encrypts LoginState with AES-256-GCM keyed from the cookie secret.
type StateSealer struct {
	aead cipher.AEAD
}

// NewStateSealer derives the key from secret.
func NewStateSealer(secret string) (*StateSealer, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &StateSealer{aead: aead}, nil
}

// Seal returns the cookie value for st.
func (s *StateSealer) Seal(st LoginState) (string, error) {
	plain, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plain, nil)), nil
}

// Open decrypts and checks expiry of a cookie value.
func (s *StateSealer) Open(value string, now time.Time) (LoginState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return LoginState{}, errBadState
	}
	plain, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], nil)
	if err != nil {
		return LoginState{}, errBadState
	}
	var st LoginState
	if err := json.Unmarshal(plain, &st); err != nil || now.After(st.ExpiresAt) {
		return LoginState{}, errBadState
	}
	return st, nil
}
