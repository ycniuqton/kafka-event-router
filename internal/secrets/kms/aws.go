package kms

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
)

// EncryptionContext binds wrapped keys to this application (AWS KMS authenticated data).
type EncryptionContext string

const (
	ContextKeyApp   EncryptionContext = "app"
	ContextValueApp EncryptionContext = "event-router"
)

// AWSAPI is the subset of the AWS KMS client the provider uses (fakeable in tests).
type AWSAPI interface {
	Encrypt(ctx context.Context, in *kms.EncryptInput, opts ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(ctx context.Context, in *kms.DecryptInput, opts ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// AWSProvider wraps data keys with an AWS KMS symmetric key.
type AWSProvider struct {
	api   AWSAPI
	keyID string
}

// NewAWSFromConfig builds the provider with the default AWS credential chain (env, IRSA, instance role).
func NewAWSFromConfig(ctx context.Context, cfg config.KMSConfig) (*AWSProvider, error) {
	if cfg.AWSKeyID == "" {
		return nil, errors.New("kms: ROUTER_AWS_KMS_KEY_ID is required for aws_kms")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("kms: aws config: %w", err)
	}
	client := kms.NewFromConfig(awsCfg, func(o *kms.Options) {
		if cfg.AWSEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		}
	})
	return NewAWS(client, cfg.AWSKeyID), nil
}

// NewAWS builds the provider over an API client.
func NewAWS(api AWSAPI, keyID string) *AWSProvider {
	return &AWSProvider{api: api, keyID: keyID}
}

// Kind implements secrets.Provider.
func (p *AWSProvider) Kind() secrets.ProviderKind { return config.SecretsAWSKMS }

// KeyID implements secrets.Provider.
func (p *AWSProvider) KeyID() string { return string(PrefixAWS) + p.keyID }

// WrapKey encrypts dek under the configured key.
func (p *AWSProvider) WrapKey(ctx context.Context, dek []byte) ([]byte, error) {
	out, err := p.api.Encrypt(ctx, &kms.EncryptInput{KeyId: aws.String(p.keyID), Plaintext: dek, EncryptionContext: p.context()})
	if err != nil {
		return nil, fmt.Errorf("kms: aws encrypt: %w", err)
	}
	return out.CiphertextBlob, nil
}

// UnwrapKey decrypts with the key recorded in keyID (old keys keep working after a config change).
func (p *AWSProvider) UnwrapKey(ctx context.Context, wrapped []byte, keyID string) ([]byte, error) {
	out, err := p.api.Decrypt(ctx, &kms.DecryptInput{KeyId: aws.String(PrefixAWS.Trim(keyID)), CiphertextBlob: wrapped,
		EncryptionContext: p.context()})
	if err != nil {
		return nil, fmt.Errorf("kms: aws decrypt: %w", err)
	}
	return out.Plaintext, nil
}

func (p *AWSProvider) context() map[string]string {
	return map[string]string{string(ContextKeyApp): string(ContextValueApp)}
}
