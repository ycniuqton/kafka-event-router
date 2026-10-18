// Package tlstest issues throwaway certificate authorities and leaf certificates for TLS/mTLS tests.
package tlstest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// PEMBlock names PEM block types.
type PEMBlock string

const (
	BlockCertificate PEMBlock = "CERTIFICATE"
	BlockPrivateKey  PEMBlock = "EC PRIVATE KEY"
)

// Validity is how long issued test certificates are valid.
type Validity time.Duration

const DefaultValidity Validity = Validity(time.Hour)

// Files are PEM files of one issued certificate.
type Files struct {
	Cert, Key string
}

// CA is a self-signed test certificate authority writing PEM files into a temp dir.
type CA struct {
	t      testing.TB
	dir    string
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	CAFile string
	serial int64
}

// NewCA creates a CA named name.
func NewCA(t testing.TB, name string) *CA {
	t.Helper()
	ca := &CA{t: t, dir: t.TempDir(), serial: 1}
	key := ca.newKey()
	tmpl := ca.template(pkix.Name{CommonName: name})
	tmpl.IsCA, tmpl.BasicConstraintsValid = true, true
	tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca.cert, _ = x509.ParseCertificate(der)
	ca.key = key
	ca.CAFile = ca.write(name+"-ca.pem", BlockCertificate, der)
	return ca
}

// Server issues a server certificate for localhost and 127.0.0.1.
func (ca *CA) Server(name string) Files {
	tmpl := ca.template(pkix.Name{CommonName: name})
	tmpl.DNSNames, tmpl.IPAddresses = []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	return ca.issue(name, tmpl)
}

// Client issues a client certificate with common name cn and an optional URI SAN.
func (ca *CA) Client(cn, uri string) Files {
	tmpl := ca.template(pkix.Name{CommonName: cn})
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if uri != "" {
		u, err := url.Parse(uri)
		if err != nil {
			ca.t.Fatal(err)
		}
		tmpl.URIs = []*url.URL{u}
	}
	return ca.issue(cn, tmpl)
}

func (ca *CA) issue(name string, tmpl *x509.Certificate) Files {
	key := ca.newKey()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		ca.t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		ca.t.Fatal(err)
	}
	return Files{Cert: ca.write(name+".pem", BlockCertificate, der), Key: ca.write(name+"-key.pem", BlockPrivateKey, keyDER)}
}

func (ca *CA) template(subject pkix.Name) *x509.Certificate {
	ca.serial++
	now := time.Now()
	return &x509.Certificate{SerialNumber: big.NewInt(ca.serial), Subject: subject,
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Duration(DefaultValidity)),
		KeyUsage: x509.KeyUsageDigitalSignature}
}

func (ca *CA) newKey() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		ca.t.Fatal(err)
	}
	return key
}

func (ca *CA) write(name string, block PEMBlock, der []byte) string {
	path := filepath.Join(ca.dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: string(block), Bytes: der}), 0o600); err != nil {
		ca.t.Fatal(err)
	}
	return path
}
