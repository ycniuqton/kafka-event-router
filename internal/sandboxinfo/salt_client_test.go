package sandboxinfo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

type fakeFetcher struct {
	paths []string
	fail  bool
	value string
}

func (f *fakeFetcher) GetJSON(_ context.Context, path string, out any) error {
	f.paths = append(f.paths, path)
	if f.fail {
		return errors.New("unreachable")
	}
	raw, _ := json.Marshal(TeamSaltResponse{TeamID: "t", ValueB64: f.value})
	return json.Unmarshal(raw, out)
}

func TestControlPlaneSaltsFetchesOncePerTeam(t *testing.T) {
	f := &fakeFetcher{value: base64.StdEncoding.EncodeToString([]byte("salt-1"))}
	s := NewControlPlaneSalts(f)
	for range 3 {
		got, err := s.Salt(context.Background(), "team a")
		if err != nil || string(got) != "salt-1" {
			t.Fatalf("salt = %q, %v", got, err)
		}
	}
	if len(f.paths) != 1 || f.paths[0] != "/internal/v1/sandbox/teams/team%20a/hash-salt" {
		t.Fatalf("paths = %v", f.paths)
	}
}

func TestControlPlaneSaltsDoesNotCacheFailures(t *testing.T) {
	f := &fakeFetcher{fail: true}
	s := NewControlPlaneSalts(f)
	if _, err := s.Salt(context.Background(), "t"); err == nil {
		t.Fatal("want error")
	}
	f.fail, f.value = false, ""
	if _, err := s.Salt(context.Background(), "t"); err == nil {
		t.Fatal("empty salt must be rejected")
	}
	if len(f.paths) != 2 {
		t.Fatalf("calls = %d", len(f.paths))
	}
}

func TestSaltSourceSelection(t *testing.T) {
	cp, proc := NewControlPlaneSalts(&fakeFetcher{}), host.SaltProvider(nil)
	reg := NewSaltSourceRegistry(cp, proc)
	loader := SettingsLoader{Lookup: func(string) (string, bool) { return "", false }}
	got, err := reg.Build(loader.SaltSource())
	if err != nil || got != host.SaltProvider(cp) {
		t.Fatalf("default source = %v, %v", got, err)
	}
	loader.Lookup = func(string) (string, bool) { return "bogus", true }
	if _, err := reg.Build(loader.SaltSource()); err == nil {
		t.Fatal("unknown source must fail")
	}
}
