package quickjs

import "testing"

func TestEmbeddedGuestChecksum(t *testing.T) {
	g := EmbeddedGuest()
	if len(g.Wasm) == 0 || len(g.SHA256) != 64 {
		t.Fatalf("guest not embedded: %d bytes, sum %q", len(g.Wasm), g.SHA256)
	}
	if err := g.Verify(); err != nil {
		t.Fatal(err)
	}
	tampered := GuestModule{Wasm: append([]byte{0}, g.Wasm...), SHA256: g.SHA256}
	if tampered.Verify() == nil {
		t.Fatal("a modified binary must fail verification")
	}
}
