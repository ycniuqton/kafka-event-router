package config

import "time"

// Transport, rate-limit, tracing, WebSocket and KMS environment keys (platform-core P2).
const (
	EnvRateLimiter          EnvKey = "ROUTER_RATE_LIMITER"
	EnvRateLimitPerMinute   EnvKey = "ROUTER_RATE_LIMIT_PER_MINUTE"
	EnvRateLimitBurst       EnvKey = "ROUTER_RATE_LIMIT_BURST"
	EnvTracingExporter      EnvKey = "ROUTER_TRACING_EXPORTER"
	EnvTracingEndpoint      EnvKey = "ROUTER_TRACING_OTLP_ENDPOINT"
	EnvTracingInsecure      EnvKey = "ROUTER_TRACING_OTLP_INSECURE"
	EnvTracingSampleRatio   EnvKey = "ROUTER_TRACING_SAMPLE_RATIO"
	EnvWSAllowedOrigins     EnvKey = "ROUTER_WS_ALLOWED_ORIGINS"
	EnvWSPingInterval       EnvKey = "ROUTER_WS_PING_INTERVAL"
	EnvWorkloadMTLSAllowed  EnvKey = "ROUTER_WORKLOAD_MTLS_ALLOWED"
	EnvControlPlaneCertFile EnvKey = "ROUTER_CONTROL_PLANE_CLIENT_CERT_FILE"
	EnvControlPlaneKeyFile  EnvKey = "ROUTER_CONTROL_PLANE_CLIENT_KEY_FILE"
	EnvAWSKMSKeyID          EnvKey = "ROUTER_AWS_KMS_KEY_ID"
	EnvAWSKMSRegion         EnvKey = "ROUTER_AWS_KMS_REGION"
	EnvAWSKMSEndpoint       EnvKey = "ROUTER_AWS_KMS_ENDPOINT"
	EnvGCPKMSKeyName        EnvKey = "ROUTER_GCP_KMS_KEY_NAME"
	EnvGCPKMSEndpoint       EnvKey = "ROUTER_GCP_KMS_ENDPOINT"
	EnvGCPCredentialsFile   EnvKey = "ROUTER_GCP_KMS_CREDENTIALS_FILE"
)

// Master-key providers added in P2 (AWS KMS and GCP Cloud KMS adapters).
const (
	SecretsAWSKMS SecretsProviderKind = "aws_kms"
	SecretsGCPKMS SecretsProviderKind = "gcp_kms"
)

// RateLimiterKind selects the rate limiter implementation.
type RateLimiterKind string

const (
	RateLimiterRedis RateLimiterKind = "redis"
	RateLimiterNone  RateLimiterKind = "none"
)

// TracingExporterKind selects the OpenTelemetry span exporter.
type TracingExporterKind string

const (
	TracingNone   TracingExporterKind = "none"
	TracingStdout TracingExporterKind = "stdout"
	TracingOTLP   TracingExporterKind = "otlp"
)

// TransportDefaults groups textual fallbacks of the P2 transport settings.
type TransportDefaults string

const (
	DefaultRateLimiter     TransportDefaults = "redis"
	DefaultTracingExporter TransportDefaults = "none"
	DefaultOTLPEndpoint    TransportDefaults = "localhost:4318"
	DefaultAWSRegion       TransportDefaults = "us-east-1"
)

// TransportNumbers groups numeric fallbacks of the P2 transport settings.
type TransportNumbers int

const (
	DefaultRateLimitPerMinute TransportNumbers = 1200
	DefaultRateLimitBurst     TransportNumbers = 300
)

// TransportRatios groups fractional fallbacks.
type TransportRatios float64

const DefaultTracingSampleRatio TransportRatios = 1.0

// TransportDurations groups duration fallbacks.
type TransportDurations time.Duration

const DefaultWSPingInterval TransportDurations = TransportDurations(30 * time.Second)

// RateLimitConfig configures per-principal API rate limiting.
type RateLimitConfig struct {
	Limiter   RateLimiterKind
	PerMinute int
	Burst     int
}

// TracingConfig configures OpenTelemetry tracing.
type TracingConfig struct {
	Exporter     TracingExporterKind
	OTLPEndpoint string
	OTLPInsecure bool
	SampleRatio  float64
}

// WebSocketConfig configures WebSocket upgrades on the public listener.
type WebSocketConfig struct {
	AllowedOrigins []string
	PingInterval   time.Duration
}

// KMSConfig configures the cloud KMS master-key providers.
type KMSConfig struct {
	AWSKeyID           string
	AWSRegion          string
	AWSEndpoint        string
	GCPKeyName         string
	GCPEndpoint        string
	GCPCredentialsFile string
}

// RequiresClientCerts reports whether the TLS settings ask for client certificates on the internal listener.
func (t TLSConfig) RequiresClientCerts() bool {
	return t.ClientCAFile != ""
}

// Enabled reports whether a server certificate is configured.
func (t TLSConfig) Enabled() bool {
	return t.CertFile != "" && t.KeyFile != ""
}
