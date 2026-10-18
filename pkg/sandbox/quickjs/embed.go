package quickjs

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

// Vendored QuickJS-ng wasm (no wasi-sdk toolchain available); see guest/THIRD_PARTY_LICENSES.txt.
//
//go:embed guest/quickjs.wasm
var quickjsWasm []byte

//go:embed guest/quickjs.wasm.sha256
var quickjsWasmSHA256 string

// GuestModule is the embedded guest binary and its pinned checksum.
type GuestModule struct {
	Wasm   []byte
	SHA256 string
}

// EmbeddedGuest returns the embedded QuickJS module.
func EmbeddedGuest() GuestModule {
	return GuestModule{Wasm: quickjsWasm, SHA256: strings.TrimSpace(quickjsWasmSHA256)}
}

// Verify checks the binary against the pinned checksum.
func (g GuestModule) Verify() error {
	sum := sha256.Sum256(g.Wasm)
	if got := hex.EncodeToString(sum[:]); got != g.SHA256 {
		return fmt.Errorf("quickjs.wasm checksum mismatch: got %s, want %s", got, g.SHA256)
	}
	return nil
}
