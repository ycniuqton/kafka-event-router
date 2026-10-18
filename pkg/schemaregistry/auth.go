// Package schemaregistry talks to Confluent-compatible schema registries of saved connections:
// per-mode authentication (none / basic / bearer), field validation and a reachability probe.
package schemaregistry

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/eventrouter/router/pkg/kafka"
)

// FieldPath is the JSON Pointer of a registry field inside the public connection resource.
type FieldPath string

const (
	PathURL      FieldPath = "/schemaRegistry/url"
	PathAuthMode FieldPath = "/schemaRegistry/auth/mode"
	PathUsername FieldPath = "/schemaRegistry/auth/username"
)

// FieldMessage holds the user-facing texts of registry validation.
type FieldMessage string

const (
	MsgURLRequired    FieldMessage = "Enter the schema registry URL."
	MsgURLInvalid     FieldMessage = "Enter a full http(s) URL, for example https://schema-registry.internal:8081."
	MsgModeInvalid    FieldMessage = "Pick none, basic or bearer authentication."
	MsgUsernameNeeded FieldMessage = "Enter the schema registry username."
	MsgPasswordNeeded FieldMessage = "Enter the schema registry password."
	MsgTokenNeeded    FieldMessage = "Enter the schema registry bearer token."
)

// AuthHeader names the HTTP authentication scheme prefixes.
type AuthHeader string

const (
	HeaderAuthorization AuthHeader = "Authorization"
	BearerPrefix        AuthHeader = "Bearer "
)

// Authenticator is the base of every registry auth mode: validation and request signing.
type Authenticator interface {
	Mode() kafka.RegistryAuthMode
	Validate(spec kafka.ConnectionSpec) []kafka.FieldError
	Apply(req *http.Request, spec kafka.ConnectionSpec)
	UsesSecret() bool
}

// AuthRegistry is the Manager selecting an Authenticator by mode.
type AuthRegistry struct {
	byMode map[kafka.RegistryAuthMode]Authenticator
}

// NewAuthRegistry returns a registry holding authenticators.
func NewAuthRegistry(auths ...Authenticator) *AuthRegistry {
	r := &AuthRegistry{byMode: map[kafka.RegistryAuthMode]Authenticator{}}
	for _, a := range auths {
		r.byMode[a.Mode()] = a
	}
	return r
}

// DefaultAuthRegistry registers none, basic and bearer.
func DefaultAuthRegistry() *AuthRegistry {
	return NewAuthRegistry(NoAuth{}, BasicAuth{}, BearerAuth{})
}

// For returns the authenticator of a mode.
func (r *AuthRegistry) For(mode kafka.RegistryAuthMode) (Authenticator, error) {
	a, ok := r.byMode[mode]
	if !ok {
		return nil, fmt.Errorf("schemaregistry: unsupported auth mode %q", mode)
	}
	return a, nil
}

// Modes lists the registered modes in stable order.
func (r *AuthRegistry) Modes() []kafka.RegistryAuthMode {
	out := make([]kafka.RegistryAuthMode, 0, len(r.byMode))
	for m := range r.byMode {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Validate checks the URL and the fields of the chosen auth mode; no registry is always valid.
func (r *AuthRegistry) Validate(spec kafka.ConnectionSpec) []kafka.FieldError {
	reg := spec.SchemaRegistry
	if reg == nil {
		return nil
	}
	errs := kafka.RequiredField{Value: reg.URL, Path: string(PathURL), Message: kafka.FieldMessage(MsgURLRequired)}.Check()
	if reg.URL != "" && !kafka.EndpointURL(reg.URL).Valid() {
		errs = append(errs, kafka.FieldError{Path: string(PathURL), Code: kafka.CodeInvalid, Message: string(MsgURLInvalid)})
	}
	auth, err := r.For(reg.Auth.ModeOrDefault())
	if err != nil {
		return append(errs, kafka.FieldError{Path: string(PathAuthMode), Code: kafka.CodeUnsupported, Message: string(MsgModeInvalid)})
	}
	return append(errs, auth.Validate(spec)...)
}

// UsesSecret reports whether the spec's registry auth mode needs the registry secret.
func (r *AuthRegistry) UsesSecret(spec kafka.ConnectionSpec) bool {
	if spec.SchemaRegistry == nil {
		return false
	}
	auth, err := r.For(spec.SchemaRegistry.Auth.ModeOrDefault())
	return err == nil && auth.UsesSecret()
}

// NoAuth sends unauthenticated requests.
type NoAuth struct{}

// Mode implements Authenticator.
func (NoAuth) Mode() kafka.RegistryAuthMode { return kafka.RegistryAuthNone }

// Validate implements Authenticator.
func (NoAuth) Validate(kafka.ConnectionSpec) []kafka.FieldError { return nil }

// Apply implements Authenticator.
func (NoAuth) Apply(*http.Request, kafka.ConnectionSpec) {}

// UsesSecret implements Authenticator.
func (NoAuth) UsesSecret() bool { return false }

// BasicAuth sends HTTP basic credentials (username + write-only password).
type BasicAuth struct{}

// Mode implements Authenticator.
func (BasicAuth) Mode() kafka.RegistryAuthMode { return kafka.RegistryAuthBasic }

// Validate implements Authenticator.
func (BasicAuth) Validate(spec kafka.ConnectionSpec) []kafka.FieldError {
	errs := kafka.RequiredField{Value: spec.SchemaRegistry.Auth.Username, Path: string(PathUsername), Message: kafka.FieldMessage(MsgUsernameNeeded)}.Check()
	return append(errs, kafka.RequiredField{Value: spec.Secret.RegistrySecret, Path: kafka.SecretPath(kafka.SecretRegistry), Message: kafka.FieldMessage(MsgPasswordNeeded)}.Check()...)
}

// Apply implements Authenticator.
func (BasicAuth) Apply(req *http.Request, spec kafka.ConnectionSpec) {
	req.SetBasicAuth(spec.SchemaRegistry.Auth.Username, spec.Secret.RegistrySecret)
}

// UsesSecret implements Authenticator.
func (BasicAuth) UsesSecret() bool { return true }

// BearerAuth sends a static bearer token (write-only).
type BearerAuth struct{}

// Mode implements Authenticator.
func (BearerAuth) Mode() kafka.RegistryAuthMode { return kafka.RegistryAuthBearer }

// Validate implements Authenticator.
func (BearerAuth) Validate(spec kafka.ConnectionSpec) []kafka.FieldError {
	return kafka.RequiredField{Value: spec.Secret.RegistrySecret, Path: kafka.SecretPath(kafka.SecretRegistry), Message: kafka.FieldMessage(MsgTokenNeeded)}.Check()
}

// Apply implements Authenticator.
func (BearerAuth) Apply(req *http.Request, spec kafka.ConnectionSpec) {
	req.Header.Set(string(HeaderAuthorization), string(BearerPrefix)+spec.Secret.RegistrySecret)
}

// UsesSecret implements Authenticator.
func (BearerAuth) UsesSecret() bool { return true }
