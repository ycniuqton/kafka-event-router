package config

import (
	"fmt"
	"strconv"
	"time"
)

// extend fills the P2 sections (rate limit, tracing, WebSocket, KMS, mTLS, client certs) of cfg.
func (l *Loader) extend(cfg *Config) error {
	rate, err := l.rateLimitConfig()
	if err != nil {
		return err
	}
	tracing, err := l.tracingConfig()
	if err != nil {
		return err
	}
	ping, err := l.duration(EnvWSPingInterval, time.Duration(DefaultWSPingInterval))
	if err != nil {
		return err
	}
	cfg.RateLimit, cfg.Tracing = rate, tracing
	cfg.WebSocket = WebSocketConfig{AllowedOrigins: l.list(EnvWSAllowedOrigins, cfg.HTTP.PublicURL), PingInterval: ping}
	cfg.Secrets.KMS = l.kmsConfig()
	cfg.Internal.MTLSAllowed = l.list(EnvWorkloadMTLSAllowed, "")
	cfg.ControlPlane.ClientCertFile = l.str(EnvControlPlaneCertFile, "")
	cfg.ControlPlane.ClientKeyFile = l.str(EnvControlPlaneKeyFile, "")
	return l.extendP3(cfg)
}

func (l *Loader) rateLimitConfig() (RateLimitConfig, error) {
	perMinute, err := l.integer(EnvRateLimitPerMinute, int(DefaultRateLimitPerMinute))
	if err != nil {
		return RateLimitConfig{}, err
	}
	burst, err := l.integer(EnvRateLimitBurst, int(DefaultRateLimitBurst))
	if err != nil {
		return RateLimitConfig{}, err
	}
	return RateLimitConfig{Limiter: RateLimiterKind(l.str(EnvRateLimiter, string(DefaultRateLimiter))),
		PerMinute: perMinute, Burst: burst}, nil
}

func (l *Loader) tracingConfig() (TracingConfig, error) {
	insecure, err := l.boolean(EnvTracingInsecure, false)
	if err != nil {
		return TracingConfig{}, err
	}
	ratio, err := l.ratio(EnvTracingSampleRatio, float64(DefaultTracingSampleRatio))
	if err != nil {
		return TracingConfig{}, err
	}
	return TracingConfig{Exporter: TracingExporterKind(l.str(EnvTracingExporter, string(DefaultTracingExporter))),
		OTLPEndpoint: l.str(EnvTracingEndpoint, string(DefaultOTLPEndpoint)), OTLPInsecure: insecure, SampleRatio: ratio}, nil
}

func (l *Loader) kmsConfig() KMSConfig {
	return KMSConfig{
		AWSKeyID:           l.str(EnvAWSKMSKeyID, ""),
		AWSRegion:          l.str(EnvAWSKMSRegion, string(DefaultAWSRegion)),
		AWSEndpoint:        l.str(EnvAWSKMSEndpoint, ""),
		GCPKeyName:         l.str(EnvGCPKMSKeyName, ""),
		GCPEndpoint:        l.str(EnvGCPKMSEndpoint, ""),
		GCPCredentialsFile: l.str(EnvGCPCredentialsFile, ""),
	}
}

func (l *Loader) integer(key EnvKey, fallback int) (int, error) {
	raw, ok := l.lookup(string(key))
	if !ok || raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	return n, nil
}

func (l *Loader) ratio(key EnvKey, fallback float64) (float64, error) {
	raw, ok := l.lookup(string(key))
	if !ok || raw == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 1 {
		return 0, fmt.Errorf("config: %s: want a number between 0 and 1, got %q", key, raw)
	}
	return f, nil
}
