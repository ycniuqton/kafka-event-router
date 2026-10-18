package httpserver

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
)

// ListenerRole tells the TLS factory which listener it configures.
type ListenerRole string

const (
	RolePublic   ListenerRole = "public"
	RoleInternal ListenerRole = "internal"
)

// ReloadInterval bounds how often certificate files are checked for rotation (cert-manager, kubelet).
type ReloadInterval time.Duration

const DefaultReloadInterval ReloadInterval = ReloadInterval(time.Minute)

// modernSuites are the TLS 1.2 cipher suites allowed (TLS 1.3 suites are fixed by Go).
var modernSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

// TLSFactory builds listener TLS configs from TLSConfig: TLS 1.2+, modern suites, hot-reloaded certificates;
// the internal listener requires and verifies client certificates when a client CA is configured (mTLS).
type TLSFactory struct {
	cfg config.TLSConfig
}

// NewTLSFactory builds the factory.
func NewTLSFactory(cfg config.TLSConfig) *TLSFactory {
	return &TLSFactory{cfg: cfg}
}

// Server returns the TLS config of a listener, or nil when no certificate is configured (plain HTTP).
func (f *TLSFactory) Server(role ListenerRole) (*tls.Config, error) {
	if !f.cfg.Enabled() {
		return nil, nil
	}
	certs, err := NewCertificateSource(f.cfg.CertFile, f.cfg.KeyFile, DefaultReloadInterval)
	if err != nil {
		return nil, err
	}
	out := &tls.Config{MinVersion: tls.VersionTLS12, CipherSuites: modernSuites, GetCertificate: certs.Get}
	if role != RoleInternal || !f.cfg.RequiresClientCerts() {
		return out, nil
	}
	pool, err := CAPool{}.Load(f.cfg.ClientCAFile)
	if err != nil {
		return nil, err
	}
	out.ClientCAs, out.ClientAuth = pool, tls.RequireAndVerifyClientCert
	return out, nil
}

// CAPool loads PEM certificate authorities.
type CAPool struct{}

// Load reads a PEM bundle into a pool; a file without certificates is an error.
func (CAPool) Load(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tls: read CA %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("tls: no certificates in " + path)
	}
	return pool, nil
}

// CertificateSource serves a key pair and reloads it when the files change on disk.
type CertificateSource struct {
	certFile, keyFile string
	interval          time.Duration
	mu                sync.Mutex
	cert              *tls.Certificate
	modTime           time.Time
	checked           time.Time
}

// NewCertificateSource loads the pair once (failing fast on bad files).
func NewCertificateSource(certFile, keyFile string, interval ReloadInterval) (*CertificateSource, error) {
	s := &CertificateSource{certFile: certFile, keyFile: keyFile, interval: time.Duration(interval)}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Get implements tls.Config.GetCertificate; a failed reload keeps serving the previous pair.
func (s *CertificateSource) Get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.checked) >= s.interval {
		s.checked = time.Now()
		if info, err := os.Stat(s.certFile); err == nil && info.ModTime().After(s.modTime) {
			_ = s.reloadLocked()
		}
	}
	return s.cert, nil
}

func (s *CertificateSource) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked()
}

func (s *CertificateSource) reloadLocked() error {
	info, err := os.Stat(s.certFile)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}
	pair, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		return fmt.Errorf("tls: load key pair: %w", err)
	}
	s.cert, s.modTime, s.checked = &pair, info.ModTime(), time.Now()
	return nil
}
