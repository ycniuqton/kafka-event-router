package kafka

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func testPEMPair(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "router"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

func paths(errs []FieldError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Path)
	}
	return out
}

func TestSecurityRegistryValidate(t *testing.T) {
	cert, key := testPEMPair(t)
	reg := DefaultSecurityRegistry()
	sasl := ConnectionSpec{SecurityMode: SecuritySASLScram512, Auth: AuthSpec{Username: "u"}, Secret: SecretBundle{Password: "p"}}
	cases := []struct {
		name string
		spec ConnectionSpec
		want []string
	}{
		{"plaintext ok", ConnectionSpec{SecurityMode: SecurityPlaintext}, nil},
		{"unknown mode", ConnectionSpec{SecurityMode: "kerberos"}, []string{"/security/mode"}},
		{"oauth missing all", ConnectionSpec{SecurityMode: SecuritySASLOAuthBearer}, []string{"/security/tokenUrl", "/security/clientId", "/secrets/clientSecret"}},
		{"oauth bad url", ConnectionSpec{SecurityMode: SecuritySASLOAuthBearer, Auth: AuthSpec{TokenURL: "idp/token", ClientID: "c"}, Secret: SecretBundle{ClientSecret: "s"}}, []string{"/security/tokenUrl"}},
		{"oauth ok", ConnectionSpec{SecurityMode: SecuritySASLOAuthBearer, Auth: AuthSpec{TokenURL: "https://idp/token", ClientID: "c"}, Secret: SecretBundle{ClientSecret: "s"}}, nil},
		{"ssl bad ca", ConnectionSpec{SecurityMode: SecuritySSL, TLS: TLSSpec{CAPEM: "nope"}}, []string{"/security/tls/caPem"}},
		{"ssl good ca", ConnectionSpec{SecurityMode: SecuritySSL, TLS: TLSSpec{CAPEM: cert}}, nil},
		{"sasl missing both", ConnectionSpec{SecurityMode: SecuritySASLPlain}, []string{"/security/username", "/secrets/password"}},
		{"scram ok", sasl, nil},
		{"scram over tls bad ca", withTLS(sasl, "bad"), []string{"/security/tls/caPem"}},
		{"mtls missing pair", ConnectionSpec{SecurityMode: SecurityMTLS}, []string{"/security/tls/clientCertPem", "/secrets/clientKeyPem"}},
		{"mtls ok", ConnectionSpec{SecurityMode: SecurityMTLS, TLS: TLSSpec{ClientCertPEM: cert}, Secret: SecretBundle{ClientKeyPEM: key}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paths(reg.Validate(tc.spec))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func withTLS(spec ConnectionSpec, ca string) ConnectionSpec {
	spec.TLS = TLSSpec{Enabled: true, CAPEM: ca}
	return spec
}

func TestSecurityOptions(t *testing.T) {
	cert, key := testPEMPair(t)
	reg := DefaultSecurityRegistry()
	specs := []ConnectionSpec{
		{SecurityMode: SecurityPlaintext},
		{SecurityMode: SecuritySSL, TLS: TLSSpec{CAPEM: cert}},
		{SecurityMode: SecurityMTLS, TLS: TLSSpec{ClientCertPEM: cert}, Secret: SecretBundle{ClientKeyPEM: key}},
		{SecurityMode: SecuritySASLPlain, Auth: AuthSpec{Username: "u"}, Secret: SecretBundle{Password: "p"}},
		{SecurityMode: SecuritySASLScram256, Auth: AuthSpec{Username: "u"}, Secret: SecretBundle{Password: "p"}, TLS: TLSSpec{Enabled: true}},
	}
	for _, spec := range specs {
		if _, err := reg.Options(spec); err != nil {
			t.Fatalf("%s: %v", spec.SecurityMode, err)
		}
	}
	bad := ConnectionSpec{SecurityMode: SecurityMTLS, TLS: TLSSpec{ClientCertPEM: cert}, Secret: SecretBundle{ClientKeyPEM: "x"}}
	if _, err := reg.Options(bad); err == nil {
		t.Fatal("expected key pair error")
	}
}

func TestSecretBundle(t *testing.T) {
	b := SecretBundle{Password: "old"}.Merge(SecretBundle{ClientKeyPEM: "k"})
	if b.Password != "old" || b.ClientKeyPEM != "k" || !b.Has(SecretClientKeyPEM) || b.Has(SecretClientSecret) {
		t.Fatalf("merge: %+v", b)
	}
	if b.String() != string(RedactedSecret) {
		t.Fatal("bundle must redact in fmt")
	}
	raw, err := SecretCodec{}.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	back, err := SecretCodec{}.Decode(raw)
	if err != nil || back != b {
		t.Fatalf("roundtrip: %+v %v", back, err)
	}
	if empty, err := (SecretCodec{}).Decode(nil); err != nil || !empty.IsEmpty() {
		t.Fatal("empty decode")
	}
}

func TestOAuthTokenCacheSharesSourcesPerCredentialSet(t *testing.T) {
	cache := NewOAuthTokenCache()
	spec := ConnectionSpec{Auth: AuthSpec{TokenURL: "https://idp/token", ClientID: "c"}, Secret: SecretBundle{ClientSecret: "s"}}
	if cache.Source(spec) != cache.Source(spec) {
		t.Fatal("same credentials must share one token source")
	}
	other := spec
	other.Secret.ClientSecret = "rotated"
	if cache.Source(spec) == cache.Source(other) {
		t.Fatal("a rotated secret must get its own token source")
	}
	if _, err := DefaultSecurityRegistry().Options(ConnectionSpec{SecurityMode: SecuritySASLOAuthBearer, Auth: spec.Auth, Secret: spec.Secret, TLS: TLSSpec{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
}
