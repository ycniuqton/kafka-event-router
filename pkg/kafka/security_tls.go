package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// TLSDefaults holds the transport constants of every TLS client.
type TLSDefaults time.Duration

const DialTimeout TLSDefaults = TLSDefaults(10 * time.Second)

// ErrBadCAPEM is returned when the CA bundle contains no certificate.
var ErrBadCAPEM = errors.New("kafka: CA PEM contains no certificate")

// TLSOptionBuilder turns a TLSSpec (+ client key) into a tls.Config and the franz-go dial option.
type TLSOptionBuilder struct{}

// Validate checks every PEM the mode uses; mTLS additionally requires the client pair.
func (b TLSOptionBuilder) Validate(spec ConnectionSpec, clientAuth bool) []FieldError {
	var errs []FieldError
	errs = append(errs, b.checkPEM(spec.TLS.CAPEM, string(PathCAPEM))...)
	if !clientAuth {
		return errs
	}
	errs = append(errs, RequiredField{Value: spec.TLS.ClientCertPEM, Path: string(PathClientCertPEM), Message: MsgClientCertRequired}.Check()...)
	errs = append(errs, RequiredField{Value: spec.Secret.ClientKeyPEM, Path: SecretPath(SecretClientKeyPEM), Message: MsgClientKeyRequired}.Check()...)
	errs = append(errs, b.checkPEM(spec.TLS.ClientCertPEM, string(PathClientCertPEM))...)
	return append(errs, b.checkPEM(spec.Secret.ClientKeyPEM, SecretPath(SecretClientKeyPEM))...)
}

// Config builds the tls.Config (TLS 1.2+, optional pinned CA, server name, client pair).
func (b TLSOptionBuilder) Config(spec ConnectionSpec, clientAuth bool) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: spec.TLS.ServerName}
	if spec.TLS.CAPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(spec.TLS.CAPEM)) {
			return nil, ErrBadCAPEM
		}
		cfg.RootCAs = pool
	}
	if !clientAuth {
		return cfg, nil
	}
	pair, err := tls.X509KeyPair([]byte(spec.TLS.ClientCertPEM), []byte(spec.Secret.ClientKeyPEM))
	if err != nil {
		return nil, err
	}
	cfg.Certificates = []tls.Certificate{pair}
	return cfg, nil
}

// Options returns the TLS dial option.
func (b TLSOptionBuilder) Options(spec ConnectionSpec, clientAuth bool) ([]kgo.Opt, error) {
	cfg, err := b.Config(spec, clientAuth)
	if err != nil {
		return nil, err
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: time.Duration(DialTimeout)}, Config: cfg}
	return []kgo.Opt{kgo.Dialer(dialer.DialContext)}, nil
}

func (b TLSOptionBuilder) checkPEM(value, path string) []FieldError {
	if value == "" {
		return nil
	}
	if block, _ := pem.Decode([]byte(value)); block == nil {
		return []FieldError{{Path: path, Code: CodeInvalid, Message: string(MsgPEMInvalid)}}
	}
	return nil
}
