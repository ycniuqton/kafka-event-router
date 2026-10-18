package testkit

import (
	"context"
	"errors"
	"testing"
)

func TestExternalProviderNotConfigured(t *testing.T) {
	t.Setenv(string(EnvRedisURL), "")
	_, _, err := NewExternalProvider().Provide(context.Background(), ServiceRedis)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	t.Setenv(string(EnvVaultAddr), "http://v:8200")
	t.Setenv(string(EnvVaultToken), "tok")
	ep, _, err := NewExternalProvider().Provide(context.Background(), ServiceVault)
	if err != nil || ep.URL != "http://v:8200" || ep.Token != "tok" {
		t.Fatalf("vault: %+v %v", ep, err)
	}
}

func TestManagerSelectsProviderFromEnv(t *testing.T) {
	t.Setenv(string(EnvProvider), string(ProviderContainer))
	if k := NewManager().Provider().Kind(); k != ProviderContainer {
		t.Fatalf("kind = %s", k)
	}
	t.Setenv(string(EnvProvider), "")
	if k := NewManager().Provider().Kind(); k != DefaultProvider {
		t.Fatalf("default kind = %s", k)
	}
}

func TestContainerSpecRender(t *testing.T) {
	env, url := DefaultContainerCatalog()[ServiceKafka].Render("127.0.0.1:5555")
	if url != "127.0.0.1:5555" {
		t.Fatalf("url = %s", url)
	}
	found := false
	for _, e := range env {
		found = found || e == "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:5555"
	}
	if !found {
		t.Fatalf("advertised listener not rendered: %v", env)
	}
}

func TestPostgresSandboxIsolatedAndDropped(t *testing.T) {
	m := NewManager()
	var schema string
	t.Run("sandbox", func(t *testing.T) {
		sb := m.Postgres(t)
		schema = sb.Schema
		if _, err := sb.DB.Pool().Exec(context.Background(), `CREATE TABLE only_here (id int)`); err != nil {
			t.Fatal(err)
		}
		var where string
		err := sb.DB.Pool().QueryRow(context.Background(),
			`SELECT table_schema FROM information_schema.tables WHERE table_name = 'only_here'`).Scan(&where)
		if err != nil || where != schema {
			t.Fatalf("table in %q (%v), want %q", where, err, schema)
		}
	})
	if schema == "" {
		t.Skip("postgres not configured")
	}
	admin := openReady(t, m.Require(t, ServicePostgres).URL)
	defer admin.Close()
	var n int
	_ = admin.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1`, schema).Scan(&n)
	if n != 0 {
		t.Fatalf("schema %s not dropped", schema)
	}
}
