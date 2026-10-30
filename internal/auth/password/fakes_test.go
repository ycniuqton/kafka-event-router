package password

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

var fastParams = Argon2Params{MemoryKiB: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func fastHashers() *HasherSet { return NewHasherSet(NewArgon2idHasher(fastParams)) }

type memCreds struct {
	mu    sync.Mutex
	users map[uuid.UUID]*Credential
}

func newMemCreds() *memCreds { return &memCreds{users: map[uuid.UUID]*Credential{}} }

func (m *memCreds) addUser(email string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.users[id] = &Credential{UserID: id, Email: email, DisplayName: strings.Split(email, "@")[0]}
	return id
}

func (m *memCreds) FindByEmail(_ context.Context, email string) (Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.users {
		if strings.EqualFold(c.Email, email) && c.Hash != "" {
			return *c, nil
		}
	}
	return Credential{}, store.ErrNotFound
}

func (m *memCreds) Get(_ context.Context, id uuid.UUID) (Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.users[id]
	if !ok || c.Hash == "" {
		return Credential{}, store.ErrNotFound
	}
	return *c, nil
}

func (m *memCreds) Set(_ context.Context, id uuid.UUID, hash string, mustChange bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.users[id]
	if !ok {
		return store.ErrNotFound
	}
	c.Hash, c.MustChange = hash, mustChange
	return nil
}

func (m *memCreds) Any(context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.users {
		if c.Hash != "" {
			return true, nil
		}
	}
	return false, nil
}

func (m *memCreds) EnsureSubject(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.users[id]; ok && c.Subject == "" {
		c.Subject = string(PasswordSubjectPrefix) + id.String()
	}
	return nil
}

type passTx struct{}

func (passTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type memRecorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (r *memRecorder) Record(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return nil
}

func (r *memRecorder) actions() []audit.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []audit.Action{}
	for _, e := range r.entries {
		out = append(out, e.Action)
	}
	return out
}

type fakeThrottle struct {
	locked   error
	failures int
	cleared  int
}

func (f *fakeThrottle) Admit(context.Context, *http.Request, string) error { return f.locked }
func (f *fakeThrottle) Failed(context.Context, string) error               { f.failures++; return nil }
func (f *fakeThrottle) Succeeded(context.Context, string) error            { f.cleared++; return nil }

func newTestCredentials(st CredentialStore, rec audit.Recorder) *Credentials {
	return NewCredentials(CredentialsDeps{Store: st, Hashers: fastHashers(), Policy: DefaultPolicy, Tx: passTx{}, Trail: NewTrail(rec)})
}
