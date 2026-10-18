package kafka

import "github.com/twmb/franz-go/pkg/kgo"

// PlaintextConfigurer is the "None" mode: no TLS, no SASL.
type PlaintextConfigurer struct{}

// Mode implements SecurityConfigurer.
func (PlaintextConfigurer) Mode() SecurityMode { return SecurityPlaintext }

// Validate implements SecurityConfigurer.
func (PlaintextConfigurer) Validate(ConnectionSpec) []FieldError { return nil }

// Options implements SecurityConfigurer.
func (PlaintextConfigurer) Options(ConnectionSpec) ([]kgo.Opt, error) { return nil, nil }

// TLSConfigurer is the TLS-only ("ssl") mode: server authentication, no client identity.
type TLSConfigurer struct {
	tls TLSOptionBuilder
}

// Mode implements SecurityConfigurer.
func (TLSConfigurer) Mode() SecurityMode { return SecuritySSL }

// Validate implements SecurityConfigurer.
func (c TLSConfigurer) Validate(spec ConnectionSpec) []FieldError { return c.tls.Validate(spec, false) }

// Options implements SecurityConfigurer.
func (c TLSConfigurer) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	return c.tls.Options(spec, false)
}
