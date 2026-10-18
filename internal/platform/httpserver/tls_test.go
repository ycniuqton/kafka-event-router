package httpserver_test

import (
	"crypto/tls"
	"testing"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/httpserver/tlstest"
)

func TestTLSFactory(t *testing.T) {
	if cfg, err := httpserver.NewTLSFactory(config.TLSConfig{}).Server(httpserver.RolePublic); cfg != nil || err != nil {
		t.Fatalf("no cert must mean plain HTTP: %v %v", cfg, err)
	}
	ca := tlstest.NewCA(t, "test")
	srv := ca.Server("api")
	f := httpserver.NewTLSFactory(config.TLSConfig{CertFile: srv.Cert, KeyFile: srv.Key, ClientCAFile: ca.CAFile})
	public, err := f.Server(httpserver.RolePublic)
	if err != nil || public.ClientAuth != tls.NoClientCert || public.MinVersion != tls.VersionTLS12 {
		t.Fatalf("public = %+v, %v", public, err)
	}
	internal, err := f.Server(httpserver.RoleInternal)
	if err != nil || internal.ClientAuth != tls.RequireAndVerifyClientCert || internal.ClientCAs == nil {
		t.Fatalf("internal = %+v, %v", internal, err)
	}
	cert, err := internal.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil || cert == nil {
		t.Fatalf("certificate: %v", err)
	}
	if _, err := httpserver.NewTLSFactory(config.TLSConfig{CertFile: srv.Cert, KeyFile: ca.CAFile}).Server(httpserver.RolePublic); err == nil {
		t.Fatal("mismatched key must fail fast")
	}
}
