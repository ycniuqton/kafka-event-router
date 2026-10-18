package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/audit"
)

// ringProvider wraps with "<keyID>|dek" and unwraps any key id it holds.
type ringProvider struct {
	current string
	keys    map[string]bool
	failOn  string
}

func (p *ringProvider) Kind() ProviderKind { return ProviderLocal }
func (p *ringProvider) KeyID() string      { return p.current }
func (p *ringProvider) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	return append([]byte(p.current+"|"), dek...), nil
}
func (p *ringProvider) UnwrapKey(_ context.Context, wrapped []byte, keyID string) ([]byte, error) {
	prefix := []byte(keyID + "|")
	if !p.keys[keyID] || !bytes.HasPrefix(wrapped, prefix) || keyID == p.failOn {
		return nil, errors.New("key unavailable")
	}
	return bytes.Clone(wrapped[len(prefix):]), nil
}

// rotationRepo is an in-memory RotationRepository over memRepo rows.
type rotationRepo struct {
	*memRepo
	raceRef Ref
}

func (r *rotationRepo) ListStale(_ context.Context, kind ProviderKind, current string, after Ref, limit int) ([]Envelope, error) {
	var out []Envelope
	for _, e := range r.rows {
		if e.Provider == kind && e.KeyID != current && e.Ref > after {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out[:min(limit, len(out))], nil
}

func (r *rotationRepo) Rewrap(_ context.Context, rw Rewrap) (bool, error) {
	e := r.rows[rw.Ref]
	if rw.Ref == r.raceRef || e.KeyID != rw.FromKeyID || !bytes.Equal(e.WrappedKey, rw.FromWrapped) {
		return false, nil
	}
	e.KeyID, e.WrappedKey = rw.ToKeyID, rw.ToWrapped
	r.rows[rw.Ref] = e
	return true, nil
}

func (r *rotationRepo) KeyUsage(context.Context) ([]KeyUsage, error) {
	counts := map[string]int{}
	for _, e := range r.rows {
		counts[e.KeyID]++
	}
	var out []KeyUsage
	for k, n := range counts {
		out = append(out, KeyUsage{Provider: ProviderLocal, KeyID: k, Count: n})
	}
	return out, nil
}

func seedEnvelopes(t *testing.T, p Provider, repo SecretRepository, n int) map[Ref]string {
	t.Helper()
	m := NewEnvelopeManager(p, repo, AESGCMCipher{})
	out := map[Ref]string{}
	for i := range n {
		plain := strings.Repeat("x", i+1)
		ref, err := m.Put(context.Background(), []byte(plain))
		if err != nil {
			t.Fatal(err)
		}
		out[ref] = plain
	}
	return out
}

func TestRotatorRewrapsWithoutTouchingPayload(t *testing.T) {
	ctx := context.Background()
	repo := &rotationRepo{memRepo: &memRepo{rows: map[Ref]Envelope{}}}
	old := &ringProvider{current: "k1", keys: map[string]bool{"k1": true}}
	plain := seedEnvelopes(t, old, repo, 5)
	before := map[Ref]Envelope{}
	for ref, e := range repo.rows {
		before[ref] = e
	}
	rec := &memRecorder{}
	next := &ringProvider{current: "k2", keys: map[string]bool{"k1": true, "k2": true}}
	rot := NewRotator(RotatorDeps{Provider: next, Repo: repo, Recorder: rec, Actor: audit.Actor{Kind: audit.ActorSystem}, PageSize: 2})
	report, err := rot.Run(ctx)
	if err != nil || report.Rotated != 5 || report.ToKeyID != "k2" || len(report.FromKeyIDs) != 1 {
		t.Fatalf("run: %+v %v", report, err)
	}
	onlyNew := &ringProvider{current: "k2", keys: map[string]bool{"k2": true}}
	m := NewEnvelopeManager(onlyNew, repo, AESGCMCipher{})
	for ref, want := range plain {
		e := repo.rows[ref]
		if !bytes.Equal(e.Ciphertext, before[ref].Ciphertext) || !bytes.Equal(e.Nonce, before[ref].Nonce) {
			t.Fatalf("%s payload changed", ref)
		}
		if got, err := m.Get(ctx, ref); err != nil || string(got) != want {
			t.Fatalf("%s readable with new key only: %q %v", ref, got, err)
		}
	}
	if len(rec.entries) != 1 || rec.entries[0].Action != audit.ActionSecretsRotated || rec.entries[0].ObjectID != "k2" {
		t.Fatalf("audit: %+v", rec.entries)
	}
	var diff RotationReport
	if err := json.Unmarshal(rec.entries[0].Diff, &diff); err != nil || diff.Rotated != 5 {
		t.Fatalf("diff: %s %v", rec.entries[0].Diff, err)
	}
	again, err := rot.Run(ctx)
	if err != nil || again.Rotated != 0 || len(rec.entries) != 1 {
		t.Fatalf("rerun not idempotent: %+v %v (audit %d)", again, err, len(rec.entries))
	}
}

func TestRotatorStopsOnFailureAndCountsRaces(t *testing.T) {
	ctx := context.Background()
	repo := &rotationRepo{memRepo: &memRepo{rows: map[Ref]Envelope{}}}
	seedEnvelopes(t, &ringProvider{current: "k1", keys: map[string]bool{"k1": true}}, repo, 2)
	seedEnvelopes(t, &ringProvider{current: "k0", keys: map[string]bool{"k0": true}}, repo, 1)
	refs := make([]Ref, 0, len(repo.rows))
	for ref := range repo.rows {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })
	repo.raceRef = refs[0]
	rec := &memRecorder{}
	next := &ringProvider{current: "k2", keys: map[string]bool{"k1": true, "k2": true}}
	report, err := NewRotator(RotatorDeps{Provider: next, Repo: repo, Recorder: rec}).Run(ctx)
	if err == nil || !errors.Is(err, ErrUnavailable) || report.Failed != string(refs[2]) || report.Raced != 1 || report.Rotated != 1 {
		t.Fatalf("partial run: %+v %v", report, err)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("partial progress must be audited: %+v", rec.entries)
	}
}

type stubLeader struct{ err error }

func (l stubLeader) Campaign(ctx context.Context) (<-chan struct{}, error) { return ctx.Done(), l.err }

type stubRotator struct{ runs int }

func (s *stubRotator) Run(context.Context) (RotationReport, error) {
	s.runs++
	return RotationReport{Rotated: 1, ToKeyID: "k2"}, nil
}
func (s *stubRotator) Usage(context.Context) ([]KeyUsage, error) { return nil, nil }

func TestRotationJobRunsOnLeaderOnly(t *testing.T) {
	rot := &stubRotator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	follower := RotationJob{Rotator: rot, Leader: stubLeader{err: errors.New("not leader")}, Log: log}
	if _, led := follower.Once(context.Background()); led || rot.runs != 0 {
		t.Fatalf("follower ran: %d", rot.runs)
	}
	leader := RotationJob{Rotator: rot, Leader: stubLeader{}, Log: log}
	if report, led := leader.Once(context.Background()); !led || rot.runs != 1 || report.Rotated != 1 {
		t.Fatalf("leader: %+v %v %d", report, led, rot.runs)
	}
}
