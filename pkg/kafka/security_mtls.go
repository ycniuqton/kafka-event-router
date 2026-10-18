package kafka

import "github.com/twmb/franz-go/pkg/kgo"

// MTLSConfigurer authenticates with a client certificate and private key over TLS.
type MTLSConfigurer struct {
	tls TLSOptionBuilder
}

// Mode implements SecurityConfigurer.
func (MTLSConfigurer) Mode() SecurityMode { return SecurityMTLS }

// Validate implements SecurityConfigurer.
func (c MTLSConfigurer) Validate(spec ConnectionSpec) []FieldError { return c.tls.Validate(spec, true) }

// Options implements SecurityConfigurer.
func (c MTLSConfigurer) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	return c.tls.Options(spec, true)
}
