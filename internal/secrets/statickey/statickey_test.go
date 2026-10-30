package statickey

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
)

func newKey(t *testing.T, fill byte) []byte {
	t.Helper()
	return bytes.Repeat([]byte{fill}, int(secrets.DataKeySize))
}

func b64(k []byte) string { return base64.StdEncoding.EncodeToString(k) }

func TestStaticKeyRoundtripAndKeyID(t *testing.T) {
	ctx := context.Background()
	key := newKey(t, 1)
	p, err := Open(config.StaticKeyConfig{Key: b64(key)})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != secrets.ProviderStaticKey || p.KeyID() != Fingerprint(key) ||
		!strings.HasPrefix(p.KeyID(), "static:") || len(p.KeyID()) != len("static:")+16 {
		t.Fatalf("kind/key id: %s %s", p.Kind(), p.KeyID())
	}
	dek := []byte("0123456789abcdef0123456789abcdef")
	wrapped, err := p.WrapKey(ctx, dek)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.UnwrapKey(ctx, wrapped, p.KeyID())
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %q %v", got, err)
	}
	if _, err := p.UnwrapKey(ctx, wrapped[:5], p.KeyID()); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("short input: %v", err)
	}
}

func TestStaticKeyWrongOrUnknownKeyFails(t *testing.T) {
	ctx := context.Background()
	a, _ := Open(config.StaticKeyConfig{Key: b64(newKey(t, 1))})
	b, _ := Open(config.StaticKeyConfig{Key: b64(newKey(t, 2))})
	wrapped, _ := a.WrapKey(ctx, newKey(t, 9))
	if _, err := b.UnwrapKey(ctx, wrapped, a.KeyID()); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unknown key id: %v", err)
	}
	if _, err := b.UnwrapKey(ctx, wrapped, b.KeyID()); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestStaticKeyRejectsBadConfig(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.key")
	_ = os.WriteFile(good, []byte(b64(newKey(t, 1))), 0o600)
	cases := map[string]struct {
		cfg  config.StaticKeyConfig
		want string
	}{
		"missing":         {config.StaticKeyConfig{}, "is required"},
		"short":           {config.StaticKeyConfig{Key: b64(make([]byte, 16))}, "must decode to 32 bytes, got 16"},
		"invalid base64":  {config.StaticKeyConfig{Key: "not*base64!"}, "not valid base64"},
		"both set":        {config.StaticKeyConfig{Key: b64(newKey(t, 1)), KeyFile: good}, "set only one"},
		"missing file":    {config.StaticKeyConfig{KeyFile: filepath.Join(dir, "none.key")}, "read"},
		"bad previous":    {config.StaticKeyConfig{Key: b64(newKey(t, 1)), PreviousKeys: []string{"xx"}}, "ROUTER_SECRETS_PREVIOUS_MASTER_KEYS"},
		"missing prev fs": {config.StaticKeyConfig{Key: b64(newKey(t, 1)), PreviousKeyFiles: []string{"/nope"}}, "ROUTER_SECRETS_PREVIOUS_MASTER_KEY_FILES"},
	}
	for name, tc := range cases {
		if _, err := Open(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %q, got %v", name, tc.want, err)
		}
	}
}

func TestStaticKeyFromFileAndRawBase64(t *testing.T) {
	key := newKey(t, 3)
	path := filepath.Join(t.TempDir(), "master.key")
	_ = os.WriteFile(path, []byte(b64(key)+"\n\t "), 0o600)
	fromFile, err := Open(config.StaticKeyConfig{KeyFile: path})
	if err != nil || fromFile.KeyID() != Fingerprint(key) {
		t.Fatalf("file: %v", err)
	}
	raw, err := Open(config.StaticKeyConfig{Key: base64.RawStdEncoding.EncodeToString(key)})
	if err != nil || raw.KeyID() != fromFile.KeyID() {
		t.Fatalf("raw base64: %v", err)
	}
}

func TestStaticKeyRegistryBuild(t *testing.T) {
	reg := secrets.NewRegistry()
	Register(reg)
	cfg := config.SecretsConfig{Provider: config.SecretsStaticKey, Static: config.StaticKeyConfig{Key: b64(newKey(t, 4))}}
	if p, err := reg.Build(context.Background(), cfg); err != nil || p.Kind() != secrets.ProviderStaticKey {
		t.Fatalf("build: %v", err)
	}
	cfg.Static.Key = b64(make([]byte, 16))
	if _, err := reg.Build(context.Background(), cfg); err == nil {
		t.Fatal("short key accepted by registry")
	}
}
