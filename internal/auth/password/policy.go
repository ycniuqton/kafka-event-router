package password

import (
	"crypto/rand"
	"math/big"
	"unicode/utf8"
)

// Policy is the minimum password policy: a length window counted in characters.
type Policy struct {
	MinLength int
	MaxLength int
}

// PolicyLimits groups the default length window (the upper bound caps hashing cost).
type PolicyLimits int

const (
	DefaultMinLength PolicyLimits = 12
	DefaultMaxLength PolicyLimits = 256
)

// DefaultPolicy is the policy every password must meet.
var DefaultPolicy = Policy{MinLength: int(DefaultMinLength), MaxLength: int(DefaultMaxLength)}

// Check rejects passwords outside the window (ErrWeakPassword at /new_password).
func (p Policy) Check(plain string) error {
	n := utf8.RuneCountInString(plain)
	if n < p.MinLength || n > p.MaxLength {
		return ErrWeakPassword
	}
	return nil
}

// GeneratorAlphabet is the character set of generated passwords (no look-alikes such as 0/O, 1/l/I).
type GeneratorAlphabet string

const SafeAlphabet GeneratorAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789-_"

// GeneratedLength is the length of generated passwords (~117 bits with the safe alphabet).
type GeneratedLength int

const DefaultGeneratedLength GeneratedLength = 20

// Generator creates random passwords shown once to an admin.
type Generator struct {
	Length GeneratedLength
}

// New returns a random password drawn uniformly from SafeAlphabet.
func (g Generator) New() (string, error) {
	length := g.Length
	if length <= 0 {
		length = DefaultGeneratedLength
	}
	out := make([]byte, length)
	limit := big.NewInt(int64(len(SafeAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = SafeAlphabet[n.Int64()]
	}
	return string(out), nil
}
