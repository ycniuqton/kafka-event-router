package preview_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/preview"
)

type countingFetcher struct {
	calls int
	paths []string
}

func (f *countingFetcher) GetJSON(_ context.Context, path string, out any) error {
	f.calls++
	f.paths = append(f.paths, path)
	body := out.(*struct {
		ValueB64 string `json:"value_b64"`
	})
	body.ValueB64 = base64.StdEncoding.EncodeToString([]byte("salt-" + path))
	return nil
}

func TestControlPlaneSaltsCachePerTeam(t *testing.T) {
	f := &countingFetcher{}
	salts := preview.NewControlPlaneSalts(f)
	a, err := salts.Salt(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := salts.Salt(context.Background(), "t1")
	other, _ := salts.Salt(context.Background(), "t2")
	if string(a) != string(again) || string(a) == string(other) || f.calls != 2 || f.paths[0] != "/internal/v1/sandbox/teams/t1/hash-salt" {
		t.Fatalf("calls=%d paths=%v", f.calls, f.paths)
	}
	reg := preview.NewSaltProviderRegistry(preview.UnreachableSalts{Err: errors.New("no token")})
	cp, err := reg.Build(preview.SaltsControlPlane)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.Salt(context.Background(), "t1"); err == nil {
		t.Fatal("unreachable control plane must fail salt lookups")
	}
	if _, err := reg.Build("nope"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}
