package secrets

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

func TestAESGCMRoundTripAndTamper(t *testing.T) {
	c := AESGCMCipher{}
	key, _ := KeyMaterial{}.New()
	nonce, ct, err := c.Seal(key, []byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.Open(key, nonce, ct)
	if err != nil || string(plain) != "hunter2" {
		t.Fatalf("round trip: %q %v", plain, err)
	}
	ct[0] ^= 0xff
	if _, err := c.Open(key, nonce, ct); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tamper: %v", err)
	}
	other, _ := KeyMaterial{}.New()
	ct[0] ^= 0xff
	if _, err := c.Open(other, nonce, ct); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong key: %v", err)
	}
}

type fakeProvider struct {
	fail    bool
	wrapped [][]byte
}

func (p *fakeProvider) Kind() ProviderKind { return ProviderLocal }
func (p *fakeProvider) KeyID() string      { return "fake:1" }
func (p *fakeProvider) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	if p.fail {
		return nil, errors.New("kms down")
	}
	out := append([]byte("w:"), dek...)
	p.wrapped = append(p.wrapped, out)
	return out, nil
}
func (p *fakeProvider) UnwrapKey(_ context.Context, wrapped []byte, keyID string) ([]byte, error) {
	if p.fail || keyID != "fake:1" {
		return nil, errors.New("unwrap failed")
	}
	return bytes.TrimPrefix(wrapped, []byte("w:")), nil
}

type memRepo struct{ rows map[Ref]Envelope }

func (m *memRepo) Insert(_ context.Context, e Envelope) error { m.rows[e.Ref] = e; return nil }
func (m *memRepo) Get(_ context.Context, r Ref) (Envelope, error) {
	e, ok := m.rows[r]
	if !ok {
		return Envelope{}, store.ErrNotFound
	}
	return e, nil
}
func (m *memRepo) Update(_ context.Context, e Envelope) error {
	if _, ok := m.rows[e.Ref]; !ok {
		return store.ErrNotFound
	}
	m.rows[e.Ref] = e
	return nil
}
func (m *memRepo) Delete(_ context.Context, r Ref) error { delete(m.rows, r); return nil }

func TestEnvelopeManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	p := &fakeProvider{}
	repo := &memRepo{rows: map[Ref]Envelope{}}
	m := NewEnvelopeManager(p, repo, AESGCMCipher{})
	ref, err := m.Put(ctx, []byte(`{"password":"p1"}`))
	if err != nil || !ref.Valid() {
		t.Fatalf("put: %q %v", ref, err)
	}
	env := repo.rows[ref]
	if bytes.Contains(env.Ciphertext, []byte("p1")) || env.KeyID != "fake:1" {
		t.Fatalf("envelope leaks plaintext or key id wrong: %+v", env)
	}
	if got, _ := m.Get(ctx, ref); string(got) != `{"password":"p1"}` {
		t.Fatalf("get: %q", got)
	}
	if err := m.Replace(ctx, ref, []byte("p2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Get(ctx, ref); string(got) != "p2" {
		t.Fatalf("replace: %q", got)
	}
	if !bytes.Equal(p.wrapped[0][2:], make([]byte, DataKeySize)) {
		t.Fatal("data key buffer not zeroed after Put")
	}
	_ = m.Delete(ctx, ref)
	if _, err := m.Get(ctx, ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := m.Replace(ctx, "sec_missing", []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replace missing: %v", err)
	}
}

func TestEnvelopeManagerProviderDown(t *testing.T) {
	m := NewEnvelopeManager(&fakeProvider{fail: true}, &memRepo{rows: map[Ref]Envelope{}}, AESGCMCipher{})
	if _, err := m.Put(context.Background(), []byte("x")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("put with provider down: %v", err)
	}
}

type memRecorder struct{ entries []audit.Entry }

func (r *memRecorder) Record(_ context.Context, e audit.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func TestAuditedReaderRecordsRead(t *testing.T) {
	ctx := context.Background()
	m := NewEnvelopeManager(&fakeProvider{}, &memRepo{rows: map[Ref]Envelope{}}, AESGCMCipher{})
	ref, _ := m.Put(ctx, []byte("v"))
	rec := &memRecorder{}
	rc := audit.RequestContext{Actor: audit.Actor{Kind: audit.ActorWorkload, Name: "worker-1"}, RequestID: "r1"}
	got, err := NewAuditedReader(m, rec).Read(ctx, ref, rc)
	if err != nil || string(got) != "v" {
		t.Fatalf("read: %q %v", got, err)
	}
	if len(rec.entries) != 1 || rec.entries[0].Action != audit.ActionSecretRead || rec.entries[0].Actor.Kind != audit.ActorWorkload ||
		rec.entries[0].ObjectID != string(ref) || rec.entries[0].Diff != nil {
		t.Fatalf("audit entry: %+v", rec.entries)
	}
}
