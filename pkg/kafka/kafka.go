package kafka

import "log/slog"

// SecurityMode is the authentication/transport mode of a saved connection.
type SecurityMode string

const (
	SecurityPlaintext       SecurityMode = "plaintext"
	SecuritySSL             SecurityMode = "ssl"
	SecurityMTLS            SecurityMode = "mtls"
	SecuritySASLPlain       SecurityMode = "sasl_plain"
	SecuritySASLScram256    SecurityMode = "sasl_scram_256"
	SecuritySASLScram512    SecurityMode = "sasl_scram_512"
	SecuritySASLOAuthBearer SecurityMode = "sasl_oauthbearer"
)

// ClusterKind selects the client factory: a real Kafka cluster or the in-memory demo cluster.
type ClusterKind string

const (
	ClusterKafka ClusterKind = "kafka"
	ClusterDemo  ClusterKind = "demo"
)

// ClientPurpose tells the factory which purpose-specific defaults to apply.
type ClientPurpose string

const (
	PurposeAdmin    ClientPurpose = "admin"
	PurposeSampler  ClientPurpose = "sampler"
	PurposeProducer ClientPurpose = "producer"
	PurposeConsumer ClientPurpose = "consumer"
)

// TLSSpec is the non-secret transport security of a connection (the client key lives in SecretBundle).
type TLSSpec struct {
	Enabled       bool   `json:"enabled"`
	CAPEM         string `json:"caPem,omitempty"`
	ServerName    string `json:"serverName,omitempty"`
	ClientCertPEM string `json:"clientCertPem,omitempty"`
}

// AuthSpec is the non-secret SASL identity of a connection.
type AuthSpec struct {
	Username string   `json:"username,omitempty"`
	ClientID string   `json:"clientId,omitempty"`
	TokenURL string   `json:"tokenUrl,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

// Equal reports whether two identities are the same (scopes compared in order).
func (a AuthSpec) Equal(other AuthSpec) bool {
	if a.Username != other.Username || a.ClientID != other.ClientID || a.TokenURL != other.TokenURL || len(a.Scopes) != len(other.Scopes) {
		return false
	}
	for i := range a.Scopes {
		if a.Scopes[i] != other.Scopes[i] {
			return false
		}
	}
	return true
}

// SecretField names one write-only secret of a connection.
type SecretField string

const (
	SecretPassword     SecretField = "password"
	SecretClientKeyPEM SecretField = "clientKeyPem"
	SecretClientSecret SecretField = "clientSecret"
	SecretRegistry     SecretField = "schemaRegistrySecret"
)

// SecretFields lists every secret field in stable order.
var SecretFields = []SecretField{SecretPassword, SecretClientKeyPEM, SecretClientSecret, SecretRegistry}

// SecretMarker is what logs show instead of a secret value.
type SecretMarker string

const RedactedSecret SecretMarker = "[redacted]"

// SecretBundle holds the decrypted secret fields; it is never logged (LogValue/String redact it).
type SecretBundle struct {
	Password       string `json:"password,omitempty"`
	ClientKeyPEM   string `json:"clientKeyPem,omitempty"`
	ClientSecret   string `json:"clientSecret,omitempty"`
	RegistrySecret string `json:"schemaRegistrySecret,omitempty"`
}

// Fields returns the bundle as a field map (empty values included).
func (b SecretBundle) Fields() map[SecretField]string {
	return map[SecretField]string{SecretPassword: b.Password, SecretClientKeyPEM: b.ClientKeyPEM, SecretClientSecret: b.ClientSecret,
		SecretRegistry: b.RegistrySecret}
}

// Has reports whether a secret field is set.
func (b SecretBundle) Has(f SecretField) bool {
	return b.Fields()[f] != ""
}

// IsEmpty reports whether no secret field is set.
func (b SecretBundle) IsEmpty() bool {
	return b == SecretBundle{}
}

// Merge overlays every non-empty field of patch onto b.
func (b SecretBundle) Merge(patch SecretBundle) SecretBundle {
	if patch.Password != "" {
		b.Password = patch.Password
	}
	if patch.ClientKeyPEM != "" {
		b.ClientKeyPEM = patch.ClientKeyPEM
	}
	if patch.ClientSecret != "" {
		b.ClientSecret = patch.ClientSecret
	}
	if patch.RegistrySecret != "" {
		b.RegistrySecret = patch.RegistrySecret
	}
	return b
}

// Only returns the bundle keeping just the given fields (unused secrets are dropped).
func (b SecretBundle) Only(fields []SecretField) SecretBundle {
	keep := map[SecretField]bool{}
	for _, f := range fields {
		keep[f] = true
	}
	values := b.Fields()
	out := SecretBundle{}
	for f, setter := range secretSetters {
		if keep[f] {
			setter(&out, values[f])
		}
	}
	return out
}

// secretSetters writes one field of a bundle.
var secretSetters = map[SecretField]func(*SecretBundle, string){
	SecretPassword:     func(b *SecretBundle, v string) { b.Password = v },
	SecretClientKeyPEM: func(b *SecretBundle, v string) { b.ClientKeyPEM = v },
	SecretClientSecret: func(b *SecretBundle, v string) { b.ClientSecret = v },
	SecretRegistry:     func(b *SecretBundle, v string) { b.RegistrySecret = v },
}

// String redacts the bundle in fmt output.
func (b SecretBundle) String() string { return string(RedactedSecret) }

// LogValue redacts the bundle in slog output.
func (b SecretBundle) LogValue() slog.Value { return slog.StringValue(string(RedactedSecret)) }

// ConnectionSpec is everything needed to open clients against one user cluster.
type ConnectionSpec struct {
	ID               string                 `json:"id"`
	Kind             ClusterKind            `json:"kind"`
	BootstrapServers []string               `json:"bootstrapServers"`
	SecurityMode     SecurityMode           `json:"securityMode"`
	TLS              TLSSpec                `json:"tls"`
	Auth             AuthSpec               `json:"auth"`
	Properties       map[PropertyKey]string `json:"properties,omitempty"`
	SchemaRegistry   *SchemaRegistrySpec    `json:"schemaRegistry,omitempty"`
	Secret           SecretBundle           `json:"secret"`
}

// RegistryAuthMode is how the router authenticates against a schema registry.
type RegistryAuthMode string

const (
	RegistryAuthNone   RegistryAuthMode = "none"
	RegistryAuthBasic  RegistryAuthMode = "basic"
	RegistryAuthBearer RegistryAuthMode = "bearer"
)

// RegistryAuthSpec is the non-secret registry identity (the password or token lives in SecretBundle).
type RegistryAuthSpec struct {
	Mode     RegistryAuthMode `json:"mode"`
	Username string           `json:"username,omitempty"`
}

// SchemaRegistrySpec is the optional schema registry of a connection.
type SchemaRegistrySpec struct {
	URL  string           `json:"url"`
	Auth RegistryAuthSpec `json:"auth"`
}

// ModeOrDefault defaults the auth mode to none.
func (a RegistryAuthSpec) ModeOrDefault() RegistryAuthMode {
	if a.Mode == "" {
		return RegistryAuthNone
	}
	return a.Mode
}

// ClusterKindOrDefault returns the spec's kind, defaulting to a real Kafka cluster.
func (s ConnectionSpec) ClusterKindOrDefault() ClusterKind {
	if s.Kind == "" {
		return ClusterKafka
	}
	return s.Kind
}
