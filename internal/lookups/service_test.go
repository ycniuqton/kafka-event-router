package lookups_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/lookups/lookupstest"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/lookup"
)

func paths(err error) string {
	var v *api.ValidationError
	if !errors.As(err, &v) {
		return ""
	}
	var b strings.Builder
	for _, e := range v.Errors {
		b.WriteString(e.Path + " ")
	}
	return b.String()
}

func draft(kind lookup.SourceKind, s lookup.Settings) lookups.Draft {
	return lookups.Draft{Name: "src", Kind: kind, EnvTag: "dev", Settings: s, Limits: lookup.DefaultLimits()}
}

func TestKindValidation(t *testing.T) {
	v := lookups.Validator{Kinds: lookups.DefaultKinds()}
	cases := []struct {
		d    lookups.Draft
		want []string
	}{
		{d: draft(lookup.KindHTTP, lookup.Settings{HTTP: &lookup.HTTPSettings{BaseURL: "https://x", Auth: lookup.HTTPAuth{Mode: lookup.AuthBasic}}}),
			want: []string{"/http/auth/username", "/secrets/password"}},
		{d: draft(lookup.KindHTTP, lookup.Settings{HTTP: &lookup.HTTPSettings{BaseURL: "https://x", Auth: lookup.HTTPAuth{Mode: "magic"}, TLS: lookup.TLSSettings{CAPEM: "x"}}}),
			want: []string{"/http/auth/mode", "/http/tls/caPem"}},
		{d: draft(lookup.KindRedis, lookup.Settings{Redis: &lookup.RedisSettings{Addr: "redis", DB: 20}}), want: []string{"/redis/addr", "/redis/db"}},
		{d: draft(lookup.KindPostgres, lookup.Settings{Postgres: &lookup.PostgresSettings{Port: 70000, SSLMode: "nope", StatementTimeoutMs: 1}}),
			want: []string{"/postgres/host", "/postgres/database", "/postgres/user", "/postgres/port", "/postgres/sslMode"}},
		{d: draft(lookup.KindStatic, lookup.Settings{Static: &lookup.StaticSettings{}}), want: []string{"/static/tableRef", "/static/version"}},
		{d: draft(lookup.KindStatic, lookup.Settings{}), want: []string{"/static"}},
		{d: draft("ftp", lookup.Settings{}), want: []string{"/kind"}},
		{d: lookups.Draft{Kind: lookup.KindStatic, EnvTag: "qa", Settings: lookup.Settings{Static: &lookup.StaticSettings{TableRef: "t", Version: 1}}},
			want: []string{"/name", "/envTag", "/limits/maxConcurrency"}},
	}
	for _, c := range cases {
		got := paths(v.Validate(v.Prepare(c.d)))
		for _, w := range c.want {
			if !strings.Contains(got, w+" ") {
				t.Fatalf("%s: want %s in %q", c.d.Kind, w, got)
			}
		}
	}
	ok := draft(lookup.KindPostgres, lookup.Settings{Postgres: &lookup.PostgresSettings{Host: "db", Database: "ref", User: "r"}})
	if err := v.Validate(v.Prepare(ok)); err != nil {
		t.Fatalf("postgres defaults (port 5432, timeout 2000) must validate: %v", err)
	}
}

func TestPrepareKeepsOnlyUsedSecrets(t *testing.T) {
	v := lookups.Validator{Kinds: lookups.DefaultKinds()}
	d := draft(lookup.KindHTTP, lookup.Settings{HTTP: &lookup.HTTPSettings{BaseURL: "https://x", Auth: lookup.HTTPAuth{Mode: lookup.AuthBearer}}})
	d.Secrets = lookup.SecretBundle{Token: "t", Password: "p"}
	if got := v.Prepare(d).Secrets; got.Password != "" || got.Token != "t" {
		t.Fatalf("secrets %+v", got)
	}
}

func TestServiceSecretsAndHealth(t *testing.T) {
	ctx := context.Background()
	mem := connectionstest.NewMemSecrets()
	repo := lookupstest.NewMemRepo()
	svc := lookups.NewService(lookups.ServiceDeps{Repo: repo, Usage: &lookupstest.MemUsage{}, Vault: lookups.NewSecretVault(mem), Tx: connectionstest.DirectTx{}})
	d := draft(lookup.KindHTTP, lookup.Settings{HTTP: &lookup.HTTPSettings{BaseURL: "https://x", Auth: lookup.HTTPAuth{Mode: lookup.AuthBearer}}})
	d.TeamID, d.Secrets = uuid.New(), lookup.SecretBundle{Token: "t1"}
	created, err := svc.Create(ctx, d)
	if err != nil || created.SecretRef == "" {
		t.Fatalf("create: %+v %v", created, err)
	}
	if err := svc.RecordHealth(ctx, created.ID, lookups.Health{Status: lookups.HealthHealthy}); err != nil {
		t.Fatal(err)
	}
	keep := d
	keep.Secrets, keep.Description, keep.Kind = lookup.SecretBundle{}, "changed", lookup.KindRedis
	updated, err := svc.Update(ctx, created.ID, 1, keep)
	if err != nil || updated.Kind != lookup.KindHTTP || updated.Health.Status != lookups.HealthHealthy {
		t.Fatalf("description-only update keeps kind and health: %+v %v", updated, err)
	}
	rotate := d
	rotate.Secrets = lookup.SecretBundle{Token: "t2"}
	updated, err = svc.Update(ctx, created.ID, 2, rotate)
	if err != nil || updated.Health.Status != lookups.HealthUnknown {
		t.Fatalf("secret replace resets health: %+v %v", updated, err)
	}
	_, spec, err := svc.Resolve(ctx, created.ID)
	if err != nil || spec.Secrets.Token != "t2" || spec.ID != created.ID.String() {
		t.Fatalf("resolve: %+v %v", spec, err)
	}
	if _, err := svc.Spec(ctx, uuid.NewString()); !errors.Is(err, lookup.ErrSourceNotFound) {
		t.Fatalf("unknown spec: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil || len(mem.Data) != 0 {
		t.Fatalf("delete drops the secret: %v (%d left)", err, len(mem.Data))
	}
}
