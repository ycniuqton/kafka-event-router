package checks_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// testPKI is an in-memory CA with one server and one client certificate (CN-53 mTLS cluster).
type testPKI struct {
	caPEM      string
	server     tls.Certificate
	clientCert string
	clientKey  string
	pool       *x509.CertPool
}

func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	caKey, caDER := issue(t, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}, nil, nil)
	ca, _ := x509.ParseCertificate(caDER)
	srvKey, srvDER := issue(t, &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "broker"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}, ca, caKey)
	cliKey, cliDER := issue(t, &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "router"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}, ca, caKey)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return testPKI{
		caPEM:      pemOf("CERTIFICATE", caDER),
		server:     tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey},
		clientCert: pemOf("CERTIFICATE", cliDER),
		clientKey:  pemOf("EC PRIVATE KEY", marshalKey(t, cliKey)),
		pool:       pool,
	}
}

func issue(t *testing.T, tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	return key, der
}

func marshalKey(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func pemOf(kind string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

// saveSecured stores a connection with full security settings and secret bundle.
func (e env) saveSecured(t *testing.T, seeds []string, sec connections.Security, secret kafka.SecretBundle) connections.Connection {
	t.Helper()
	ctx := context.Background()
	ref, err := e.vault.Store(ctx, secret)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.repo.Create(ctx, connections.Connection{
		TeamID: uuid.New(), Name: "c-" + uuid.NewString(), EnvTag: connections.EnvDev, BootstrapServers: seeds, Kind: kafka.ClusterKafka,
		Security: sec, SecretRef: ref, Health: connections.Health{Status: connections.HealthUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestScram512ClusterPassesFullChecklist(t *testing.T) {
	kc := cluster(t, kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-512", "admin", "secret"))
	e := newEnv()
	sec := connections.Security{Mode: kafka.SecuritySASLScram512, Auth: kafka.AuthSpec{Username: "admin"}}
	c := e.saveSecured(t, kc.ListenAddrs(), sec, kafka.SecretBundle{Password: "secret"})
	sink, saved := e.run(t, c.ID, connections.TestOptions{ReadTopic: "orders", WriteTopic: "orders"})
	if !sink.Result.OK || saved.Health.Status != connections.HealthHealthy {
		t.Fatalf("result %+v events %+v", sink.Result, sink.Events)
	}
}

func TestScramWrongMechanismFailsAuthentication(t *testing.T) {
	kc := cluster(t, kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-512", "admin", "secret"))
	e := newEnv()
	sec := connections.Security{Mode: kafka.SecuritySASLScram256, Auth: kafka.AuthSpec{Username: "admin"}}
	c := e.saveSecured(t, kc.ListenAddrs(), sec, kafka.SecretBundle{Password: "secret"})
	sink, saved := e.run(t, c.ID, connections.TestOptions{})
	failure := saved.Health.LastFailure
	if sink.Result.OK || saved.Health.Status != connections.HealthAuthFailed || failure == nil || failure.Step != connections.StepAuthenticated {
		t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
	}
	if failure.Reason != kafka.ReasonAuthFailed && failure.Reason != kafka.ReasonSASLMismatch {
		t.Fatalf("reason %q", failure.Reason)
	}
}

func TestMTLSClusterRequiresTheClientCertificate(t *testing.T) {
	pki := newTestPKI(t)
	serverTLS := &tls.Config{Certificates: []tls.Certificate{pki.server}, ClientCAs: pki.pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
	kc := cluster(t, kfake.TLS(serverTLS))
	e := newEnv()
	mtls := connections.Security{Mode: kafka.SecurityMTLS, TLS: kafka.TLSSpec{Enabled: true, CAPEM: pki.caPEM, ClientCertPEM: pki.clientCert}}
	ok := e.saveSecured(t, kc.ListenAddrs(), mtls, kafka.SecretBundle{ClientKeyPEM: pki.clientKey})
	if sink, saved := e.run(t, ok.ID, connections.TestOptions{ReadTopic: "orders", WriteTopic: "orders"}); !sink.Result.OK {
		t.Fatalf("mTLS should pass: health %+v events %+v", saved.Health, sink.Events)
	}
	tlsOnly := connections.Security{Mode: kafka.SecuritySSL, TLS: kafka.TLSSpec{Enabled: true, CAPEM: pki.caPEM}}
	noCert := e.saveSecured(t, kc.ListenAddrs(), tlsOnly, kafka.SecretBundle{})
	sink, saved := e.run(t, noCert.ID, connections.TestOptions{})
	if sink.Result.OK || saved.Health.Status == connections.HealthHealthy {
		t.Fatalf("a client without certificate must fail: health %+v", saved.Health)
	}
}
