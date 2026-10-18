package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/platform/config"
)

func fakeTransit(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "root" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var in transitRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := transitResponse{}
		switch r.URL.Path {
		case "/v1/transit/encrypt/event-router":
			out.Data.Ciphertext = "vault:v1:" + in.Plaintext
		case "/v1/transit/decrypt/event-router":
			out.Data.Plaintext = strings.TrimPrefix(in.Ciphertext, "vault:v1:")
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func TestVaultTransitWrapUnwrap(t *testing.T) {
	srv := fakeTransit(t)
	defer srv.Close()
	p, err := New(config.VaultConfig{Addr: srv.URL, Token: "root", TransitKey: "event-router", Mount: "transit"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wrapped, err := p.WrapKey(ctx, []byte("dek-bytes"))
	if err != nil || !strings.HasPrefix(string(wrapped), "vault:v1:") {
		t.Fatalf("wrap: %q %v", wrapped, err)
	}
	got, err := p.UnwrapKey(ctx, wrapped, p.KeyID())
	if err != nil || string(got) != "dek-bytes" {
		t.Fatalf("unwrap: %q %v", got, err)
	}
	bad, _ := New(config.VaultConfig{Addr: srv.URL, Token: "wrong", TransitKey: "event-router", Mount: "transit"}, srv.Client())
	if _, err := bad.WrapKey(ctx, []byte("x")); err == nil {
		t.Fatal("bad token accepted")
	}
	if _, err := New(config.VaultConfig{}, nil); err == nil {
		t.Fatal("empty config accepted")
	}
}
