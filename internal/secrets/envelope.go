package secrets

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// EnvelopeManager is the only Manager implementation: AES-GCM payload, data key wrapped by the Provider.
type EnvelopeManager struct {
	provider Provider
	repo     SecretRepository
	cipher   DataCipher
	keys     KeyMaterial
}

// NewEnvelopeManager builds the manager.
func NewEnvelopeManager(provider Provider, repo SecretRepository, cipher DataCipher) *EnvelopeManager {
	return &EnvelopeManager{provider: provider, repo: repo, cipher: cipher}
}

// Put encrypts plaintext under a new data key and stores the envelope (inside the caller's tx).
func (m *EnvelopeManager) Put(ctx context.Context, plaintext []byte) (Ref, error) {
	e, err := m.seal(ctx, NewRef(), plaintext)
	if err != nil {
		return "", err
	}
	if err := m.repo.Insert(ctx, e); err != nil {
		return "", err
	}
	return e.Ref, nil
}

// Get unwraps the data key and decrypts the payload.
func (m *EnvelopeManager) Get(ctx context.Context, ref Ref) ([]byte, error) {
	e, err := m.repo.Get(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dek, err := m.provider.UnwrapKey(ctx, e.WrappedKey, e.KeyID)
	if err != nil {
		return nil, ErrUnavailable.Wrap(err)
	}
	defer m.keys.Zero(dek)
	return m.cipher.Open(dek, e.Nonce, e.Ciphertext)
}

// Replace re-encrypts the secret under a fresh data key, keeping its reference.
func (m *EnvelopeManager) Replace(ctx context.Context, ref Ref, plaintext []byte) error {
	e, err := m.seal(ctx, ref, plaintext)
	if err != nil {
		return err
	}
	err = m.repo.Update(ctx, e)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// Delete removes the envelope.
func (m *EnvelopeManager) Delete(ctx context.Context, ref Ref) error {
	return m.repo.Delete(ctx, ref)
}

func (m *EnvelopeManager) seal(ctx context.Context, ref Ref, plaintext []byte) (Envelope, error) {
	dek, err := m.keys.New()
	if err != nil {
		return Envelope{}, err
	}
	defer m.keys.Zero(dek)
	nonce, ct, err := m.cipher.Seal(dek, plaintext)
	if err != nil {
		return Envelope{}, err
	}
	wrapped, err := m.provider.WrapKey(ctx, dek)
	if err != nil {
		return Envelope{}, ErrUnavailable.Wrap(err)
	}
	return Envelope{Ref: ref, Provider: m.provider.Kind(), KeyID: m.provider.KeyID(), WrappedKey: wrapped, Nonce: nonce, Ciphertext: ct}, nil
}

// AuditedReader reads secrets for workloads and records secret.read_by_service for each read.
type AuditedReader struct {
	manager  Manager
	recorder audit.Recorder
}

// NewAuditedReader wraps a Manager with read auditing.
func NewAuditedReader(manager Manager, recorder audit.Recorder) *AuditedReader {
	return &AuditedReader{manager: manager, recorder: recorder}
}

// Read decrypts ref and records the read with the given request context.
func (a *AuditedReader) Read(ctx context.Context, ref Ref, rc audit.RequestContext) ([]byte, error) {
	plain, err := a.manager.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	entry := rc.Apply(audit.Entry{Action: audit.ActionSecretRead, Object: audit.ObjectSecret, ObjectID: string(ref)})
	if err := a.recorder.Record(ctx, entry); err != nil {
		clear(plain)
		return nil, err
	}
	return plain, nil
}
