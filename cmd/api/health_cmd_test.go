package main

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
)

func TestHealthCommandVerifiesAgainstInternalCA(t *testing.T) {
	ready := true
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}
	if err := os.WriteFile(caFile, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &HealthCommand{out: io.Discard, lookup: func(k string) (string, bool) {
		return caFile, k == string(config.EnvTLSClientCAFile)
	}}
	if code := cmd.Run([]string{srv.URL + "/readyz"}); code != app.ExitOK {
		t.Fatalf("ready: exit %d", code)
	}
	ready = false
	if code := cmd.Run([]string{srv.URL + "/readyz"}); code != app.ExitFailure {
		t.Fatalf("not ready: exit %d", code)
	}
	untrusted := &HealthCommand{out: io.Discard, lookup: func(string) (string, bool) { return "", false }}
	if code := untrusted.Run([]string{srv.URL + "/readyz"}); code != app.ExitFailure {
		t.Fatal("an unknown CA must fail the probe")
	}
	if code := cmd.Run(nil); code != app.ExitFailure {
		t.Fatal("missing url must fail")
	}
}
