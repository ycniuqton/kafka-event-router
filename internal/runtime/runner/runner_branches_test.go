// Multi-branch delivery (RR-101, RR-105): one source on a plaintext cluster, branch "scram" to a SASL/SCRAM
// cluster and branch "mtls" to an mTLS cluster; every key is counted per branch, per-partition order is
// checked per branch and the produced series carry their branch label.
package runner_test

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
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
)

const twoBranchRoute = `{"apiVersion":"router/v1","name":"fanout","source":{"connectionId":"src","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"branches":[
 {"id":"scram","filter":null,"destination":{"connectionId":"scram","topic":"out-scram","key":{"mode":"keep"}}},
 {"id":"mtls","filter":{"type":"cond","field":"value.keep","op":"eq","value":true},"destination":{"connectionId":"mtls","topic":"out-mtls","key":{"mode":"keep"}}}]}`

// branchCluster is the volume and shape of the two-branch scenario.
var branchCluster = struct {
	Partitions, Keys, PerKey int
	ScramUser, ScramPass     string
}{Partitions: 3, Keys: 12, PerKey: 25, ScramUser: "router", ScramPass: "s3cret"}

// branchPKI is an in-memory CA with a broker and a client certificate.
type branchPKI struct {
	caPEM, clientCert, clientKey string
	server                       tls.Certificate
	pool                         *x509.CertPool
}

func newBranchPKI(t *testing.T) branchPKI {
	t.Helper()
	window := func(c *x509.Certificate) *x509.Certificate {
		c.NotBefore, c.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		return c
	}
	caKey, caDER := issueCert(t, window(&x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, IsCA: true,
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
	return branchPKI{caPEM: pemBlock("CERTIFICATE", caDER), clientCert: pemBlock("CERTIFICATE", cliDER), clientKey: pemBlock("EC PRIVATE KEY", keyDER),
		server: tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}, pool: pool}
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

// branchClusters starts the plaintext source, the SCRAM and the mTLS destination clusters.
func branchClusters(t *testing.T, pki branchPKI) (src, scramC, mtlsC *kfake.Cluster) {
	t.Helper()
	start := func(opts ...kfake.Opt) *kfake.Cluster {
		c, err := kfake.NewCluster(append([]kfake.Opt{kfake.NumBrokers(1)}, opts...)...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		return c
	}
	n := int32(branchCluster.Partitions)
	src = start(kfake.SeedTopics(n, "in"))
	scramC = start(kfake.SeedTopics(n, "out-scram"), kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-512", branchCluster.ScramUser, branchCluster.ScramPass))
	serverTLS := &tls.Config{Certificates: []tls.Certificate{pki.server}, ClientCAs: pki.pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
	mtlsC = start(kfake.SeedTopics(n, "out-mtls"), kfake.TLS(serverTLS))
	return src, scramC, mtlsC
}

func twoBranchVersion(t *testing.T, pki branchPKI, src, scramC, mtlsC *kfake.Cluster) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(twoBranchRoute))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		t.Fatal(err)
	}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, engine.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	conns := map[string]kafka.ConnectionSpec{
		"src": {ID: "src", BootstrapServers: src.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext},
		"scram": {ID: "scram", BootstrapServers: scramC.ListenAddrs(), SecurityMode: kafka.SecuritySASLScram512,
			Auth: kafka.AuthSpec{Username: branchCluster.ScramUser}, Secret: kafka.SecretBundle{Password: branchCluster.ScramPass}},
		"mtls": {ID: "mtls", BootstrapServers: mtlsC.ListenAddrs(), SecurityMode: kafka.SecurityMTLS,
			TLS: kafka.TLSSpec{Enabled: true, CAPEM: pki.caPEM, ClientCertPEM: pki.clientCert}, Secret: kafka.SecretBundle{ClientKeyPEM: pki.clientKey}},
	}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "fan", VersionID: "v1", VersionNo: 1}, RouteName: "fanout", Config: cfg, Pipeline: p, Connections: conns}
}

// seqEvent is one generated source event: key k, per-key sequence, keep flag for the mtls branch.
type seqEvent struct {
	K    string `json:"k"`
	Seq  int    `json:"seq"`
	Keep bool   `json:"keep"`
}

func produceSequences(t *testing.T, src *kfake.Cluster) (keptByMTLS int) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(src.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for seq := 0; seq < branchCluster.PerKey; seq++ {
		for k := 0; k < branchCluster.Keys; k++ {
			ev := seqEvent{K: fmt.Sprintf("key-%d", k), Seq: seq, Keep: (seq+k)%2 == 0}
			if ev.Keep {
				keptByMTLS++
			}
			raw, _ := json.Marshal(ev)
			if err := cl.ProduceSync(context.Background(), &kgo.Record{Key: []byte(ev.K), Value: raw}).FirstErr(); err != nil {
				t.Fatal(err)
			}
		}
	}
	return keptByMTLS
}

// branchOutput is what one destination topic received, per partition in log order.
type branchOutput map[int32][]seqEvent

func consumeBranch(t *testing.T, opts []kgo.Opt, topic string, want int) branchOutput {
	t.Helper()
	cl, err := kgo.NewClient(append(opts, kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))...)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	out, n := branchOutput{}, 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for n < want && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			var ev seqEvent
			if json.Unmarshal(r.Value, &ev) == nil {
				out[r.Partition] = append(out[r.Partition], ev)
				n++
			}
		})
	}
	return out
}

// verify checks no loss (every expected key/seq present) and per-partition order per key.
func (o branchOutput) verify(t *testing.T, branch string, expected func(seqEvent) bool) {
	t.Helper()
	seen := map[string]map[int]bool{}
	for p, evs := range o {
		last := map[string]int{}
		for _, ev := range evs {
			if prev, ok := last[ev.K]; ok && ev.Seq < prev {
				t.Fatalf("%s: partition %d: %s seq %d after %d (order broken)", branch, p, ev.K, ev.Seq, prev)
			}
			last[ev.K] = ev.Seq
			if seen[ev.K] == nil {
				seen[ev.K] = map[int]bool{}
			}
			seen[ev.K][ev.Seq] = true
		}
	}
	for seq := 0; seq < branchCluster.PerKey; seq++ {
		for k := 0; k < branchCluster.Keys; k++ {
			ev := seqEvent{K: fmt.Sprintf("key-%d", k), Seq: seq, Keep: (seq+k)%2 == 0}
			if expected(ev) && !seen[ev.K][ev.Seq] {
				t.Fatalf("%s: lost %s seq %d", branch, ev.K, ev.Seq)
			}
		}
	}
}

func TestTwoBranchRouteDeliversToScramAndMTLSClustersInOrder(t *testing.T) {
	pki := newBranchPKI(t)
	src, scramC, mtlsC := branchClusters(t, pki)
	total := branchCluster.Keys * branchCluster.PerKey
	kept := produceSequences(t, src)
	reg := prometheus.NewRegistry()
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	v := twoBranchVersion(t, pki, src, scramC, mtlsC)
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	scramOpts := []kgo.Opt{kgo.SeedBrokers(scramC.ListenAddrs()...),
		kgo.SASL(scram.Auth{User: branchCluster.ScramUser, Pass: branchCluster.ScramPass}.AsSha512Mechanism())}
	clientCert, err := tls.X509KeyPair([]byte(pki.clientCert), []byte(pki.clientKey))
	if err != nil {
		t.Fatal(err)
	}
	mtlsOpts := []kgo.Opt{kgo.SeedBrokers(mtlsC.ListenAddrs()...),
		kgo.DialTLSConfig(&tls.Config{RootCAs: pki.pool, Certificates: []tls.Certificate{clientCert}, MinVersion: tls.VersionTLS12})}
	consumeBranch(t, scramOpts, "out-scram", total).verify(t, "scram", func(seqEvent) bool { return true })
	consumeBranch(t, mtlsOpts, "out-mtls", kept).verify(t, "mtls", func(ev seqEvent) bool { return ev.Keep })
	eventually(t, "commit of every source record", func() bool { return committed(t, src, "router.fan") == int64(total) })
	assertBranchSeries(t, reg, map[string]float64{"scram": float64(total), "mtls": float64(kept)}, float64(total))
}

// assertBranchSeries checks produced per branch and that the record-level consumed series counts each record once.
func assertBranchSeries(t *testing.T, reg *prometheus.Registry, produced map[string]float64, consumed float64) {
	t.Helper()
	eventually(t, "per-branch produced series", func() bool {
		got := counterByBranch(t, reg, "router_events_produced_total")
		for b, want := range produced {
			if got[b] != want {
				return false
			}
		}
		return true
	})
	got := counterByBranch(t, reg, "router_events_consumed_total")
	if got["scram"] != consumed || got["mtls"] != 0 {
		t.Fatalf("consumed must be counted once on the primary branch: %v", got)
	}
}

func counterByBranch(t *testing.T, reg *prometheus.Registry, name string) map[string]float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "branch" {
					out[l.GetValue()] += m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

func TestRoutesOnTheSameDestinationShareOneProducer(t *testing.T) {
	pki := newBranchPKI(t)
	src, scramC, mtlsC := branchClusters(t, pki)
	total := branchCluster.Keys * branchCluster.PerKey
	produceSequences(t, src)
	deps := testDeps(clients.NewOpenerManager(clients.NewFranzOpener()), "w1", nil)
	m := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, &haltRecorder{}, deps.Log)
	first := twoBranchVersion(t, pki, src, scramC, mtlsC)
	second := twoBranchVersion(t, pki, src, scramC, mtlsC)
	second.Ref = domain.VersionRef{RouteID: "fan-2", VersionID: "v1", VersionNo: 1}
	for _, v := range []runner.CompiledVersion{first, second} {
		if err := m.Start(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, "both routes committed", func() bool {
		return committed(t, src, "router.fan") == int64(total) && committed(t, src, "router.fan-2") == int64(total)
	})
	if n := deps.Producers.Size(); n != 2 {
		t.Fatalf("two routes on the same two destinations must share 2 producers, got %d", n)
	}
	m.StopAll(context.Background())
	if n := deps.Producers.Size(); n != 0 {
		t.Fatalf("producers left open after stop: %d", n)
	}
}
