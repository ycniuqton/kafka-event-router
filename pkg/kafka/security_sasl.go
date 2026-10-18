package kafka

import (
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// ScramVariant is the hash function of a SCRAM mechanism.
type ScramVariant string

const (
	ScramSHA256 ScramVariant = "SCRAM-SHA-256"
	ScramSHA512 ScramVariant = "SCRAM-SHA-512"
)

// ScramProfile binds a variant to its security mode and franz-go mechanism.
type ScramProfile struct {
	Mode      SecurityMode
	Mechanism func(scram.Auth) sasl.Mechanism
}

// ScramProfiles is the domain table of supported SCRAM variants.
var ScramProfiles = map[ScramVariant]ScramProfile{
	ScramSHA256: {Mode: SecuritySASLScram256, Mechanism: scram.Auth.AsSha256Mechanism},
	ScramSHA512: {Mode: SecuritySASLScram512, Mechanism: scram.Auth.AsSha512Mechanism},
}

// SASLCredentials validates username/password and adds the optional TLS transport.
type SASLCredentials struct {
	tls TLSOptionBuilder
}

// Validate checks the SASL identity and, when TLS is enabled, the TLS PEMs.
func (c SASLCredentials) Validate(spec ConnectionSpec) []FieldError {
	errs := RequiredField{Value: spec.Auth.Username, Path: string(PathUsername), Message: MsgUsernameRequired}.Check()
	errs = append(errs, RequiredField{Value: spec.Secret.Password, Path: SecretPath(SecretPassword), Message: MsgPasswordRequired}.Check()...)
	if spec.TLS.Enabled {
		errs = append(errs, c.tls.Validate(spec, false)...)
	}
	return errs
}

// Options returns the SASL option plus the TLS dialer when TLS is enabled.
func (c SASLCredentials) Options(spec ConnectionSpec, mechanism sasl.Mechanism) ([]kgo.Opt, error) {
	opts := []kgo.Opt{kgo.SASL(mechanism)}
	if !spec.TLS.Enabled {
		return opts, nil
	}
	tlsOpts, err := c.tls.Options(spec, false)
	if err != nil {
		return nil, err
	}
	return append(opts, tlsOpts...), nil
}

// SASLPlainConfigurer is SASL/PLAIN, optionally over TLS.
type SASLPlainConfigurer struct {
	tls TLSOptionBuilder
}

// Mode implements SecurityConfigurer.
func (SASLPlainConfigurer) Mode() SecurityMode { return SecuritySASLPlain }

// Validate implements SecurityConfigurer.
func (c SASLPlainConfigurer) Validate(spec ConnectionSpec) []FieldError {
	return SASLCredentials(c).Validate(spec)
}

// Options implements SecurityConfigurer.
func (c SASLPlainConfigurer) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	mech := plain.Auth{User: spec.Auth.Username, Pass: spec.Secret.Password}.AsMechanism()
	return SASLCredentials(c).Options(spec, mech)
}

// SASLScramConfigurer is SASL/SCRAM (256 or 512), optionally over TLS.
type SASLScramConfigurer struct {
	creds   SASLCredentials
	profile ScramProfile
}

// NewSASLScramConfigurer builds the configurer of one SCRAM variant.
func NewSASLScramConfigurer(variant ScramVariant, tls TLSOptionBuilder) SASLScramConfigurer {
	return SASLScramConfigurer{creds: SASLCredentials{tls: tls}, profile: ScramProfiles[variant]}
}

// Mode implements SecurityConfigurer.
func (c SASLScramConfigurer) Mode() SecurityMode { return c.profile.Mode }

// Validate implements SecurityConfigurer.
func (c SASLScramConfigurer) Validate(spec ConnectionSpec) []FieldError {
	return c.creds.Validate(spec)
}

// Options implements SecurityConfigurer.
func (c SASLScramConfigurer) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	mech := c.profile.Mechanism(scram.Auth{User: spec.Auth.Username, Pass: spec.Secret.Password})
	return c.creds.Options(spec, mech)
}
