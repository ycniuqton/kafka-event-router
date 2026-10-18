// Package kms holds the cloud master-key providers: AWS KMS and GCP Cloud KMS. Both wrap per-secret
// data keys behind secrets.Provider and are selected by ROUTER_SECRETS_PROVIDER (aws_kms | gcp_kms).
package kms

import (
	"context"
	"strings"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
)

// KeyIDPrefix marks which provider produced a stored key id.
type KeyIDPrefix string

const (
	PrefixAWS KeyIDPrefix = "aws_kms:"
	PrefixGCP KeyIDPrefix = "gcp_kms:"
)

// Trim returns the provider-native key reference of a stored key id.
func (p KeyIDPrefix) Trim(keyID string) string {
	return strings.TrimPrefix(keyID, string(p))
}

// Register adds both KMS providers to a registry.
func Register(reg *secrets.Registry) {
	reg.Register(config.SecretsAWSKMS, func(ctx context.Context, cfg config.SecretsConfig) (secrets.Provider, error) {
		return NewAWSFromConfig(ctx, cfg.KMS)
	})
	reg.Register(config.SecretsGCPKMS, func(ctx context.Context, cfg config.SecretsConfig) (secrets.Provider, error) {
		return NewGCPFromConfig(ctx, cfg.KMS)
	})
}
