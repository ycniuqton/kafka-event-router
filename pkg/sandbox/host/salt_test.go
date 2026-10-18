package host

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

type memSecrets struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memSecrets) Put(_ context.Context, b []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := fmt.Sprintf("ref-%d", len(m.data))
	m.data[ref] = b
	return ref, nil
}

func (m *memSecrets) Get(_ context.Context, ref string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[ref], nil
}

type memRefs struct {
	mu   sync.Mutex
	refs map[string]string
	sets int
}

func (m *memRefs) HashSaltRef(_ context.Context, team string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.refs[team]
	return r, ok, nil
}

func (m *memRefs) SetHashSaltRefIfEmpty(_ context.Context, team, ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sets++
	if r, ok := m.refs[team]; ok {
		return r, nil
	}
	m.refs[team] = ref
	return ref, nil
}

func TestStoreSaltProviderGeneratesOnceAndCaches(t *testing.T) {
	secrets, refs := &memSecrets{data: map[string][]byte{}}, &memRefs{refs: map[string]string{}}
	p := NewStoreSaltProvider(secrets, refs)
	ctx := context.Background()
	a, err := p.Salt(ctx, "team-a")
	if err != nil || len(a) != int(SaltSizeDefault) {
		t.Fatalf("salt = %x, %v", a, err)
	}
	again, _ := p.Salt(ctx, "team-a")
	b, _ := p.Salt(ctx, "team-b")
	if string(a) != string(again) || string(a) == string(b) {
		t.Fatal("salts must be stable per team and differ between teams")
	}
	if refs.sets != 2 {
		t.Fatalf("refs set %d times, want once per team", refs.sets)
	}
	fresh := NewStoreSaltProvider(secrets, refs)
	if reloaded, _ := fresh.Salt(ctx, "team-a"); string(reloaded) != string(a) {
		t.Fatal("a new process must read the stored salt")
	}
}

func TestStoreSaltProviderKeepsConcurrentWinner(t *testing.T) {
	secrets := &memSecrets{data: map[string][]byte{"winner": []byte("w")}}
	refs := &memRefs{refs: map[string]string{}}
	racing := &racingRefs{memRefs: refs}
	p := NewStoreSaltProvider(secrets, racing)
	got, err := p.Salt(context.Background(), "t")
	if err != nil || string(got) != "w" {
		t.Fatalf("salt = %q, %v; want the concurrent winner", got, err)
	}
}

type racingRefs struct{ *memRefs }

func (r *racingRefs) SetHashSaltRefIfEmpty(ctx context.Context, team, ref string) (string, error) {
	r.refs[team] = "winner"
	return r.memRefs.SetHashSaltRefIfEmpty(ctx, team, ref)
}
