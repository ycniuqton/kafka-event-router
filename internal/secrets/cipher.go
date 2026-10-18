package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// KeySize is the AES-256 key size in bytes for data keys and local master keys.
type KeySize int

const DataKeySize KeySize = 32

// ErrDecrypt is returned when ciphertext fails authentication (tampering or wrong key).
var ErrDecrypt = errors.New("secrets: decryption failed")

// DataCipher seals payloads with a per-secret data key.
type DataCipher interface {
	Seal(dek, plaintext []byte) (nonce, ciphertext []byte, err error)
	Open(dek, nonce, ciphertext []byte) ([]byte, error)
}

// AESGCMCipher is the AES-256-GCM DataCipher.
type AESGCMCipher struct{}

// Seal encrypts plaintext with a random nonce.
func (AESGCMCipher) Seal(dek, plaintext []byte) ([]byte, []byte, error) {
	aead, err := AESGCMCipher{}.aead(dek)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plaintext, nil), nil
}

// Open decrypts and authenticates ciphertext.
func (AESGCMCipher) Open(dek, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := AESGCMCipher{}.aead(dek)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, ErrDecrypt
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// KeyMaterial generates and erases key bytes.
type KeyMaterial struct{}

// New returns a fresh random data key.
func (KeyMaterial) New() ([]byte, error) {
	k := make([]byte, DataKeySize)
	_, err := rand.Read(k)
	return k, err
}

// Zero overwrites b so key material does not linger in memory.
func (KeyMaterial) Zero(b []byte) {
	clear(b)
}

func (AESGCMCipher) aead(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
