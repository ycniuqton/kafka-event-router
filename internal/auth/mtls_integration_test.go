package auth_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/auth/workload"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/httpserver/tlstest"
)

// mtlsServer serves the workload name on an internal listener configured like production (TLSFactory, mTLS).
func mtlsServer(t *testing.T, ca *tlstest.CA, allowed []string) *httptest.Server {
	t.Helper()
	srvCert := ca.Server("api")
	tlsCfg, err := httpserver.NewTLSFactory(config.TLSConfig{CertFile: srvCert.Cert, KeyFile: srvCert.Key, ClientCAFile: ca.CAFile}).
		Server(httpserver.RoleInternal)
	if err != nil {
		t.Fatal(err)
	}
	authn := workload.NewMTLSAuthenticator(allowed)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := authn.Authenticate(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, p.WorkloadName)
	}))
	srv.Listener = tls.NewListener(srv.Listener, tlsCfg)
	srv.Start()
	srv.URL = strings.Replace(srv.URL, "http://", "https://", 1)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, cp config.ControlPlaneConfig, url string) (int, string, error) {
	t.Helper()
	client, err := controlplane.TransportFactory{}.ClientFor(cp)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

// TestMTLSWorkloadAuth (PC-72): pinned CA, client certificates, allow-list, rogue CA and missing certificate.
func TestMTLSWorkloadAuth(t *testing.T) {
	ca, rogue := tlstest.NewCA(t, "event-router"), tlstest.NewCA(t, "rogue")
	worker := ca.Client("worker", "spiffe://event-router/worker")
	preview := ca.Client("preview", "")
	intruder := rogue.Client("worker", "spiffe://event-router/worker")
	srv := mtlsServer(t, ca, []string{"spiffe://event-router/worker", "preview"})
	strict := mtlsServer(t, ca, []string{"spiffe://event-router/worker"})
	pin := func(f tlstest.Files) config.ControlPlaneConfig {
		return config.ControlPlaneConfig{CAFile: ca.CAFile, ClientCertFile: f.Cert, ClientKeyFile: f.Key}
	}
	if code, body, err := call(t, pin(worker), srv.URL); err != nil || code != 200 || body != "spiffe://event-router/worker" {
		t.Fatalf("worker: %d %q %v", code, body, err)
	}
	if code, body, _ := call(t, pin(preview), srv.URL); code != 200 || body != "preview" {
		t.Fatalf("preview by CN: %d %q", code, body)
	}
	if code, _, _ := call(t, pin(preview), strict.URL); code != http.StatusUnauthorized {
		t.Fatalf("allow-list must reject preview: %d", code)
	}
	if _, _, err := call(t, config.ControlPlaneConfig{CAFile: ca.CAFile}, srv.URL); err == nil {
		t.Fatal("handshake without a client certificate must fail")
	}
	if _, _, err := call(t, pin(intruder), srv.URL); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("rogue CA must fail the handshake: %v", err)
	}
}
