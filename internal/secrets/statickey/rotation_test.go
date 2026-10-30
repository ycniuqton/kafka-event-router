package statickey

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
)

// memStore is an in-memory SecretRepository + RotationRepository.
type memStore struct {
	rows map[secrets.Ref]secrets.Envelope
}

func (m *memStore) Insert(_ context.Context, e secrets.Envelope) error { m.rows[e.Ref] = e; return nil }
func (m *memStore) Get(_ context.Context, r secrets.Ref) (secrets.Envelope, error) {
	e, ok := m.rows[r]
	if !ok {
		return secrets.Envelope{}, store.ErrNotFound
	}
	return e, nil
}
func (m *memStore) Update(_ context.Context, e secrets.Envelope) error { m.rows[e.Ref] = e; return nil }
func (m *memStore) Delete(_ context.Context, r secrets.Ref) error      { delete(m.rows, r); return nil }

func (m *memStore) ListStale(_ context.Context, kind secrets.ProviderKind, current string, after secrets.Ref, limit int) ([]secrets.Envelope, error) {
	var out []secrets.Envelope
	for _, e := range m.rows {
		if e.Provider == kind && e.KeyID != current && e.Ref > after {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out[:min(limit, len(out))], nil
}

func (m *memStore) Rewrap(_ context.Context, rw secrets.Rewrap) (bool, error) {
	e := m.rows[rw.Ref]
	if e.KeyID != rw.FromKeyID || !bytes.Equal(e.WrappedKey, rw.FromWrapped) {
		return false, nil
	}
	e.KeyID, e.WrappedKey = rw.ToKeyID, rw.ToWrapped
	m.rows[rw.Ref] = e
	return true, nil
}

func (m *memStore) KeyUsage(context.Context) ([]secrets.KeyUsage, error) { return nil, nil }

type nopRecorder struct{ n int }

func (r *nopRecorder) Record(context.Context, audit.Entry) error { r.n++; return nil }

func TestStaticKeyPreviousKeysAndRotation(t *testing.T) {
	ctx := context.Background()
	oldB64, newB64 := b64(newKey(t, 1)), b64(newKey(t, 2))
	repo := &memStore{rows: map[secrets.Ref]secrets.Envelope{}}
	old, _ := Open(config.StaticKeyConfig{Key: oldB64})
	ref, err := secrets.NewEnvelopeManager(old, repo, secrets.AESGCMCipher{}).Put(ctx, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	ring, err := Open(config.StaticKeyConfig{Key: newB64, PreviousKeys: []string{oldB64}})
	if err != nil || ring.KeyID() == old.KeyID() {
		t.Fatalf("ring: %v", err)
	}
	if got, err := secrets.NewEnvelopeManager(ring, repo, secrets.AESGCMCipher{}).Get(ctx, ref); err != nil || string(got) != "pw" {
		t.Fatalf("get via previous key: %q %v", got, err)
	}
	alone, _ := Open(config.StaticKeyConfig{Key: newB64})
	if _, err := secrets.NewEnvelopeManager(alone, repo, secrets.AESGCMCipher{}).Get(ctx, ref); err == nil {
		t.Fatal("old envelope decrypted without previous key")
	}
	rec := &nopRecorder{}
	rot := secrets.NewRotator(secrets.RotatorDeps{Provider: ring, Repo: repo, Recorder: rec, Actor: audit.Actor{Kind: audit.ActorSystem}})
	report, err := rot.Run(ctx)
	if err != nil || report.Rotated != 1 || report.ToKeyID != ring.KeyID() || repo.rows[ref].KeyID != ring.KeyID() || rec.n != 1 {
		t.Fatalf("rotate: %+v %v", report, err)
	}
	if got, err := secrets.NewEnvelopeManager(alone, repo, secrets.AESGCMCipher{}).Get(ctx, ref); err != nil || string(got) != "pw" {
		t.Fatalf("get after rotation without previous key: %q %v", got, err)
	}
}
