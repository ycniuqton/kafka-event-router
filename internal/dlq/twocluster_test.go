package dlq

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
)

// twoClusterRoute reads from a SASL/SCRAM cluster (DLQ there too) and writes to an mTLS cluster (DL-66).
const twoClusterRoute = `{
  "apiVersion": "router/v1", "name": "payments",
  "source": {"connectionId": "scram", "topics": ["payments"], "format": {"type": "json"}},
  "branches": [{"id": "main",
    "transform": {"steps": [{"kind": "rename", "from": "value.amt", "to": "value.amount"}]},
    "destination": {"connectionId": "mtls", "topic": "payments-public", "format": {"type": "json"}}}],
  "errorPolicy": {"onError": "dlq", "dlqTopic": "router.dlq.payments"}
}`

// testCA is an in-memory CA with a broker certificate (127.0.0.1) and a client certificate.
type testCA struct {
	caPEM, clientCert, clientKey string
	server                       tls.Certificate
	pool                         *x509.CertPool
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	window := func(c *x509.Certificate) *x509.Certificate {
		c.NotBefore, c.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		return c
	}
	caKey, caDER := issueCert(t, window(&x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true}), nil, nil)
	ca, _ := x509.ParseCertificate(caDER)
	srvKey, srvDER := issueCert(t, window(&x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "broker"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}), ca, caKey)
	cliKey, cliDER := issueCert(t, window(&x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "router"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}), ca, caKey)
	keyDER, err := x509.MarshalECPrivateKey(cliKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return testCA{
		caPEM: pemBlock("CERTIFICATE", caDER), clientCert: pemBlock("CERTIFICATE", cliDER), clientKey: pemBlock("EC PRIVATE KEY", keyDER),
		server: tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}, pool: pool,
	}
}

func issueCert(t *testing.T, tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*ecdsa.PrivateKey, []byte) {
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

func pemBlock(kind string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

// staticSpecs resolves the two test connections to their client specs.
type staticSpecs map[string]kafka.ConnectionSpec

func (s staticSpecs) Spec(_ context.Context, id string) (kafka.ConnectionSpec, error) {
	spec, ok := s[id]
	if !ok {
		return kafka.ConnectionSpec{}, errors.New("unknown connection")
	}
	return spec, nil
}

func twoClusters(t *testing.T) (ClientOpener, testCA) {
	t.Helper()
	pki := newTestCA(t)
	scram, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-256", "router", "s3cret"),
		kfake.SeedTopics(2, "payments", "router.dlq.payments"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scram.Close)
	serverTLS := &tls.Config{Certificates: []tls.Certificate{pki.server}, ClientCAs: pki.pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
	mtls, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.TLS(serverTLS), kfake.SeedTopics(2, "payments-public"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mtls.Close)
	specs := staticSpecs{
		"scram": {ID: "scram", BootstrapServers: scram.ListenAddrs(), SecurityMode: kafka.SecuritySASLScram256,
			Auth: kafka.AuthSpec{Username: "router"}, Secret: kafka.SecretBundle{Password: "s3cret"}},
		"mtls": {ID: "mtls", BootstrapServers: mtls.ListenAddrs(), SecurityMode: kafka.SecurityMTLS,
			TLS: kafka.TLSSpec{Enabled: true, CAPEM: pki.caPEM, ClientCertPEM: pki.clientCert}, Secret: kafka.SecretBundle{ClientKeyPEM: pki.clientKey}},
	}
	factory := kafka.NewFranzClientFactory(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog(), kafka.DefaultPurposeDefaults())
	return SpecClientOpener{Specs: specs, Options: factory}, pki
}

func produceTwoClusterDLQ(t *testing.T, opener ClientOpener, values []string) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "scram", kafka.PurposeProducer)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for i, v := range values {
		raw := record.Codec{}.Encode(record.Record{Key: []byte("k"), Value: []byte(v),
			Failure: record.Failure{RouteID: "r2", VersionNo: 1, Branch: "main", Step: "step-0", ErrorType: record.ErrorStep, Message: "m", FailedAt: time.Now().Add(-time.Duration(i) * time.Second)},
			Source:  record.SourcePosition{Topic: "payments", Offset: int64(i)}})
		hs := make([]kgo.RecordHeader, 0, len(raw.Headers))
		for _, h := range raw.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		if err := cl.ProduceSync(context.Background(), &kgo.Record{Topic: "router.dlq.payments", Key: raw.Key, Value: raw.Value, Headers: hs}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetryAcrossScramSourceAndMTLSDestination(t *testing.T) {
	opener, _ := twoClusters(t)
	produceTwoClusterDLQ(t, opener, []string{`{"amt":1}`, `{"amt":2}`})
	routes := fakeRoutes{route: RouteInfo{ID: "r2", Name: "payments", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(twoClusterRoute)}}}
	m, err := NewAssembly(AssemblyDeps{
		Settings: Settings{StateBackend: StateBackendMemory, Executor: ExecutorInProcess, Scripts: ScriptsNone},
		Routes:   routes, Clients: opener, Compiler: compile.NewDefaultCompiler(), Scripts: NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: NoAuditTrail{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	page, err := m.List(ctx, "r2", Query{})
	if err != nil || len(page.Items) != 2 || page.Items[0].StepLabel != "Step 1 · Rename field" {
		t.Fatalf("list over SCRAM: %v %+v", err, page)
	}
	report, err := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r2", Selection{IDs: []string{page.Items[0].ID, page.Items[1].ID}})
	if err != nil || report.Counts()[OutcomeSent] != 2 {
		t.Fatalf("retry into mTLS: %v %+v", err, report)
	}
	assertMTLSDestination(t, opener, 2)
	if left, _ := m.List(ctx, "r2", Query{}); len(left.Items) != 0 {
		t.Fatalf("retried events still listed: %d", len(left.Items))
	}
}

func assertMTLSDestination(t *testing.T, opener ClientOpener, want int) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "mtls", kafka.PurposeConsumer, kgo.ConsumeTopics("payments-public"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	got := 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for got < want && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			if !strings.Contains(string(r.Value), `"amount"`) {
				t.Errorf("not transformed: %s", r.Value)
			}
			got++
		})
	}
	if got != want {
		t.Fatalf("mTLS destination got %d, want %d", got, want)
	}
}
