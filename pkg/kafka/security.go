package kafka

import (
	"fmt"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
)

// FieldPath is the JSON Pointer of a connection field inside the public resource shape.
type FieldPath string

const (
	PathBootstrapServers FieldPath = "/bootstrapServers"
	PathSecurityMode     FieldPath = "/security/mode"
	PathUsername         FieldPath = "/security/username"
	PathCAPEM            FieldPath = "/security/tls/caPem"
	PathClientCertPEM    FieldPath = "/security/tls/clientCertPem"
	PathTokenURL         FieldPath = "/security/tokenUrl"
	PathSecrets          FieldPath = "/secrets/"
	PathProperties       FieldPath = "/properties/"
)

// SecretPath returns the pointer of one secret field.
func SecretPath(f SecretField) string { return string(PathSecrets) + string(f) }

// PropertyPath returns the pointer of one advanced property row.
func PropertyPath(key string) string {
	return string(PathProperties) + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// FieldCode classifies a field error.
type FieldCode string

const (
	CodeRequired    FieldCode = "required"
	CodeInvalid     FieldCode = "invalid"
	CodeUnknown     FieldCode = "unknown"
	CodeUnsupported FieldCode = "unsupported"
)

// FieldError points at one invalid connection field.
type FieldError struct {
	Path    string
	Code    FieldCode
	Message string
}

// FieldMessage holds the user-facing texts of security validation.
type FieldMessage string

const (
	MsgUsernameRequired   FieldMessage = "Enter the SASL username."
	MsgPasswordRequired   FieldMessage = "Enter the SASL password."
	MsgClientCertRequired FieldMessage = "Add the client certificate (.pem) for mTLS."
	MsgClientKeyRequired  FieldMessage = "Add the client private key (.pem) for mTLS."
	MsgPEMInvalid         FieldMessage = "This is not a valid PEM block. Paste or drop the .pem file content."
	MsgModeUnsupported    FieldMessage = "This security mode is not supported yet. Pick another mode."
)

// SecurityConfigurer is the base of every security mode: validation and franz-go options.
type SecurityConfigurer interface {
	Mode() SecurityMode
	Validate(spec ConnectionSpec) []FieldError
	Options(spec ConnectionSpec) ([]kgo.Opt, error)
}

// SecurityRegistry is the Manager selecting a configurer by mode; it never switches on modes.
type SecurityRegistry struct {
	byMode map[SecurityMode]SecurityConfigurer
}

// NewSecurityRegistry returns an empty registry.
func NewSecurityRegistry() *SecurityRegistry {
	return &SecurityRegistry{byMode: map[SecurityMode]SecurityConfigurer{}}
}

// DefaultSecurityRegistry registers every supported security mode (OAuth shares one token cache).
func DefaultSecurityRegistry() *SecurityRegistry {
	r := NewSecurityRegistry()
	tls := TLSOptionBuilder{}
	r.Register(NewOAuthBearerConfigurer(tls, NewOAuthTokenCache()))
	r.Register(PlaintextConfigurer{})
	r.Register(TLSConfigurer{tls: tls})
	r.Register(MTLSConfigurer{tls: tls})
	r.Register(SASLPlainConfigurer{tls: tls})
	r.Register(NewSASLScramConfigurer(ScramSHA256, tls))
	r.Register(NewSASLScramConfigurer(ScramSHA512, tls))
	return r
}

// Register adds or replaces the configurer of its mode.
func (r *SecurityRegistry) Register(c SecurityConfigurer) {
	r.byMode[c.Mode()] = c
}

// For returns the configurer of a mode.
func (r *SecurityRegistry) For(mode SecurityMode) (SecurityConfigurer, error) {
	c, ok := r.byMode[mode]
	if !ok {
		return nil, fmt.Errorf("kafka: unsupported security mode %q", mode)
	}
	return c, nil
}

// Modes lists the registered modes in stable order.
func (r *SecurityRegistry) Modes() []SecurityMode {
	out := make([]SecurityMode, 0, len(r.byMode))
	for m := range r.byMode {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Validate checks the spec against its mode; an unknown mode is itself a field error.
func (r *SecurityRegistry) Validate(spec ConnectionSpec) []FieldError {
	c, err := r.For(spec.SecurityMode)
	if err != nil {
		return []FieldError{{Path: string(PathSecurityMode), Code: CodeUnsupported, Message: string(MsgModeUnsupported)}}
	}
	return c.Validate(spec)
}

// Options returns the franz-go options of the spec's mode.
func (r *SecurityRegistry) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	c, err := r.For(spec.SecurityMode)
	if err != nil {
		return nil, err
	}
	return c.Options(spec)
}

// RequiredField is one "must not be empty" rule used by configurers.
type RequiredField struct {
	Value   string
	Path    string
	Message FieldMessage
}

// Check returns a required-field error when the value is empty.
func (f RequiredField) Check() []FieldError {
	if f.Value != "" {
		return nil
	}
	return []FieldError{{Path: f.Path, Code: CodeRequired, Message: string(f.Message)}}
}
