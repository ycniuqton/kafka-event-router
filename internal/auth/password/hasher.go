package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Algorithm names a password hashing scheme; it is the first field of the PHC-encoded hash.
type Algorithm string

const AlgorithmArgon2id Algorithm = "argon2id"

// Hasher is the base of every password hashing scheme (encoded hashes carry their own parameters).
type Hasher interface {
	Algorithm() Algorithm
	Hash(plain string) (string, error)
	Verify(encoded, plain string) (bool, error)
}

// ErrMalformedHash is returned for stored hashes that cannot be parsed.
var ErrMalformedHash = errors.New("password: malformed hash")

// Argon2Params are the argon2id cost parameters (OWASP baseline: 19 MiB, 2 passes, 1 lane).
type Argon2Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	SaltLen   uint32
	KeyLen    uint32
}

// DefaultArgon2Params is the cost used for new hashes.
var DefaultArgon2Params = Argon2Params{MemoryKiB: 19 * 1024, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32}

// PHCFormat is the encoding of argon2id hashes: $argon2id$v=19$m=…,t=…,p=…$salt$key (raw std base64).
type PHCFormat string

const (
	PHCArgon2Format PHCFormat = "$%s$v=%d$m=%d,t=%d,p=%d$%s$%s"
	PHCParamsFormat PHCFormat = "m=%d,t=%d,p=%d"
	PHCVersionField PHCFormat = "v=%d"
	PHCSeparator    PHCFormat = "$"
)

// PHCFieldCount is the number of `$`-separated fields of an argon2id PHC string (leading empty field included).
type PHCFieldCount int

const Argon2Fields PHCFieldCount = 6

// Argon2idHasher hashes with argon2id and verifies in constant time.
type Argon2idHasher struct {
	params Argon2Params
}

// NewArgon2idHasher builds the hasher with params for new hashes.
func NewArgon2idHasher(params Argon2Params) *Argon2idHasher {
	return &Argon2idHasher{params: params}
}

// Algorithm implements Hasher.
func (h *Argon2idHasher) Algorithm() Algorithm { return AlgorithmArgon2id }

// Hash returns the PHC string of plain under a fresh random salt.
func (h *Argon2idHasher) Hash(plain string) (string, error) {
	salt := make([]byte, h.params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	p := h.params
	key := argon2.IDKey([]byte(plain), salt, p.Time, p.MemoryKiB, p.Threads, p.KeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf(string(PHCArgon2Format), AlgorithmArgon2id, argon2.Version, p.MemoryKiB, p.Time, p.Threads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify recomputes the key with the parameters stored in encoded and compares in constant time.
func (h *Argon2idHasher) Verify(encoded, plain string) (bool, error) {
	parsed, err := parseArgon2(encoded)
	if err != nil {
		return false, err
	}
	p := parsed.params
	key := argon2.IDKey([]byte(plain), parsed.salt, p.Time, p.MemoryKiB, p.Threads, uint32(len(parsed.key)))
	return subtle.ConstantTimeCompare(key, parsed.key) == 1, nil
}

// argon2Hash is a parsed PHC string.
type argon2Hash struct {
	params Argon2Params
	salt   []byte
	key    []byte
}

func parseArgon2(encoded string) (argon2Hash, error) {
	fields := strings.Split(encoded, string(PHCSeparator))
	if len(fields) != int(Argon2Fields) || Algorithm(fields[1]) != AlgorithmArgon2id {
		return argon2Hash{}, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(fields[2], string(PHCVersionField), &version); err != nil || version != argon2.Version {
		return argon2Hash{}, ErrMalformedHash
	}
	var out argon2Hash
	if _, err := fmt.Sscanf(fields[3], string(PHCParamsFormat), &out.params.MemoryKiB, &out.params.Time, &out.params.Threads); err != nil {
		return argon2Hash{}, ErrMalformedHash
	}
	salt, errSalt := base64.RawStdEncoding.DecodeString(fields[4])
	key, errKey := base64.RawStdEncoding.DecodeString(fields[5])
	if errSalt != nil || errKey != nil || len(key) == 0 {
		return argon2Hash{}, ErrMalformedHash
	}
	out.salt, out.key = salt, key
	return out, nil
}

// HasherSet is the Manager of hashing schemes: new hashes use the current one, stored hashes verify with
// the scheme named in their PHC prefix (so the scheme can change without invalidating stored passwords).
type HasherSet struct {
	current Hasher
	byAlg   map[Algorithm]Hasher
}

// NewHasherSet builds the set; current is also registered for verification.
func NewHasherSet(current Hasher, others ...Hasher) *HasherSet {
	s := &HasherSet{current: current, byAlg: map[Algorithm]Hasher{}}
	for _, h := range append(others, current) {
		s.byAlg[h.Algorithm()] = h
	}
	return s
}

// Hash hashes plain with the current scheme.
func (s *HasherSet) Hash(plain string) (string, error) {
	return s.current.Hash(plain)
}

// Verify checks plain against encoded with the scheme it was hashed with.
func (s *HasherSet) Verify(encoded, plain string) (bool, error) {
	fields := strings.SplitN(encoded, string(PHCSeparator), 3)
	if len(fields) < 2 {
		return false, ErrMalformedHash
	}
	h, ok := s.byAlg[Algorithm(fields[1])]
	if !ok {
		return false, ErrMalformedHash
	}
	return h.Verify(encoded, plain)
}
