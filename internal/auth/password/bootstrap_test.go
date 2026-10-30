package password

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

type memAccounts struct {
	creds    *memCreds
	promoted map[uuid.UUID]bool
}

func (m *memAccounts) Lock(context.Context) error { return nil }
func (m *memAccounts) FindOrCreate(_ context.Context, email, _ string) (uuid.UUID, error) {
	for id, c := range m.creds.users {
		if strings.EqualFold(c.Email, email) {
			return id, nil
		}
	}
	return m.creds.addUser(email), nil
}
func (m *memAccounts) PromoteWorkspaceAdmin(_ context.Context, id uuid.UUID) error {
	m.promoted[id] = true
	return nil
}

type memTeams struct{ id uuid.UUID }

func (t memTeams) EnsureByName(context.Context, string) (uuid.UUID, error) { return t.id, nil }

type memBindings struct{ roles map[uuid.UUID]auth.Role }

func (b memBindings) Ensure(_ context.Context, user, _ uuid.UUID, role auth.Role) (bool, error) {
	b.roles[user] = role
	return true, nil
}

func newSeeder(cfg config.PasswordLoginConfig) (*AdminSeeder, *memCreds, *memAccounts, memBindings, *memRecorder) {
	st, rec := newMemCreds(), &memRecorder{}
	acc, binds := &memAccounts{creds: st, promoted: map[uuid.UUID]bool{}}, memBindings{roles: map[uuid.UUID]auth.Role{}}
	return &AdminSeeder{Creds: newTestCredentials(st, rec), Accounts: acc, Teams: memTeams{id: uuid.New()}, Bindings: binds,
		Tx: passTx{}, Recorder: rec, Config: cfg, TeamName: "default", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, st, acc, binds, rec
}

func TestSeederCreatesAdminOnce(t *testing.T) {
	s, st, acc, binds, rec := newSeeder(config.PasswordLoginConfig{BootstrapEmail: " Admin@Example.com ", BootstrapPassword: "first-start-password"})
	ctx := context.Background()
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}
	cred, err := st.FindByEmail(ctx, "admin@example.com")
	if err != nil || !cred.MustChange || !acc.promoted[cred.UserID] || binds.roles[cred.UserID] != auth.RoleAdmin {
		t.Fatalf("cred=%+v err=%v", cred, err)
	}
	s.Config.BootstrapPassword = "another-password-x"
	if err := s.Run(ctx); err != nil || len(st.users) != 1 || len(rec.entries) != 1 {
		t.Fatalf("second start must not seed again: err=%v users=%d audit=%d", err, len(st.users), len(rec.entries))
	}
	if ok, _ := s.Creds.Hashers().Verify(cred.Hash, "first-start-password"); !ok {
		t.Fatal("seed password must verify")
	}
}

func TestSeederSkipsOrRefuses(t *testing.T) {
	s, st, _, _, _ := newSeeder(config.PasswordLoginConfig{})
	if err := s.Run(context.Background()); err != nil || len(st.users) != 0 {
		t.Fatalf("no email: err=%v", err)
	}
	s, _, _, _, _ = newSeeder(config.PasswordLoginConfig{BootstrapEmail: "a@example.com"})
	if err := s.Run(context.Background()); !errors.Is(err, ErrNoBootstrapPassword) {
		t.Fatalf("missing password: %v", err)
	}
	s, _, _, _, _ = newSeeder(config.PasswordLoginConfig{BootstrapEmail: "a@example.com", BootstrapPassword: "short"})
	if err := s.Run(context.Background()); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak password: %v", err)
	}
}

func TestSeederReadsPasswordFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(path, []byte("password-from-a-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, st, _, _, _ := newSeeder(config.PasswordLoginConfig{BootstrapEmail: "a@example.com", BootstrapPassword: "ignored-env-value", BootstrapPasswordFile: path})
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	cred, _ := st.FindByEmail(context.Background(), "a@example.com")
	if ok, _ := s.Creds.Hashers().Verify(cred.Hash, "password-from-a-file"); !ok {
		t.Fatal("the file wins over the variable")
	}
}
