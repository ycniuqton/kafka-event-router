package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalProviderCreatesKeyAndWraps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "master.key")
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", info, err)
	}
	ctx := context.Background()
	dek := []byte("0123456789abcdef0123456789abcdef")
	wrapped, err := p.WrapKey(ctx, dek)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil || reopened.KeyID() != p.KeyID() {
		t.Fatalf("reopen: %v %s/%s", err, reopened.KeyID(), p.KeyID())
	}
	got, err := reopened.UnwrapKey(ctx, wrapped, p.KeyID())
	if err != nil || string(got) != string(dek) {
		t.Fatalf("unwrap: %q %v", got, err)
	}
	if _, err := reopened.UnwrapKey(ctx, wrapped, "local:other"); err == nil {
		t.Fatal("foreign key id accepted")
	}
	if _, err := reopened.UnwrapKey(ctx, wrapped[:5], p.KeyID()); err == nil {
		t.Fatal("short input accepted")
	}
}

func TestLocalProviderRejectsBadKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.key")
	_ = os.WriteFile(path, []byte("not-hex"), 0o600)
	if _, err := Open(path); err == nil {
		t.Fatal("bad key file accepted")
	}
}

func TestLocalProviderUnwrapsWithPreviousKeys(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old.key"), filepath.Join(dir, "new.key")
	ctx := context.Background()
	old, err := Open(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	dek := []byte("0123456789abcdef0123456789abcdef")
	wrapped, _ := old.WrapKey(ctx, dek)
	ring, err := Open(newPath, oldPath)
	if err != nil || ring.KeyID() == old.KeyID() {
		t.Fatalf("ring: %v", err)
	}
	if got, err := ring.UnwrapKey(ctx, wrapped, old.KeyID()); err != nil || string(got) != string(dek) {
		t.Fatalf("previous key unwrap: %q %v", got, err)
	}
	if _, err := Open(newPath, filepath.Join(dir, "missing.key")); err == nil {
		t.Fatal("missing previous key file accepted (must not be generated)")
	}
	alone, _ := Open(newPath)
	if _, err := alone.UnwrapKey(ctx, wrapped, old.KeyID()); err == nil {
		t.Fatal("old key id unwrapped without the previous key file")
	}
}

func TestLocalKeyIDFormatUnchanged(t *testing.T) {
	key := make([]byte, 32)
	if got := Fingerprint(key); got != "local:66687aadf862bd77" {
		t.Fatalf("local key id changed (existing envelopes would stop decrypting): %s", got)
	}
}
