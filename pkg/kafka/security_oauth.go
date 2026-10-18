package kafka

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/oauth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuthLimits bounds the client-credentials token source.
type OAuthLimits int

const (
	TokenRequestTimeout OAuthLimits = OAuthLimits(10 * time.Second)
	MaxCachedTokenSets  OAuthLimits = 256
)

// EndpointScheme lists the accepted schemes of HTTP endpoints (token endpoint, schema registry).
type EndpointScheme string

const (
	SchemeHTTPS EndpointScheme = "https"
	SchemeHTTP  EndpointScheme = "http"
)

// OAuth field messages.
const (
	MsgTokenURLRequired     FieldMessage = "Enter the token endpoint URL of your identity provider."
	MsgTokenURLInvalid      FieldMessage = "Enter a full http(s) URL, for example https://idp.example.com/oauth2/token."
	MsgClientIDRequired     FieldMessage = "Enter the OAuth client ID."
	MsgClientSecretRequired FieldMessage = "Enter the OAuth client secret."
)

// PathClientID is the pointer of the OAuth client id.
const PathClientID FieldPath = "/security/clientId"

// EndpointURL validates HTTP endpoint URLs entered by users.
type EndpointURL string

// Valid reports whether the URL is an absolute http(s) URL with a host.
func (u EndpointURL) Valid() bool {
	parsed, err := url.Parse(strings.TrimSpace(string(u)))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == string(SchemeHTTPS) || parsed.Scheme == string(SchemeHTTP)
}

// TokenCacheKey holds the separators of the credential-set cache key.
type TokenCacheKey string

const (
	KeyFieldSeparator TokenCacheKey = "\x00"
	ScopeSeparator    TokenCacheKey = " "
)

// OAuthTokenCache shares one caching client-credentials token source per credential set across clients.
type OAuthTokenCache struct {
	mu      sync.Mutex
	sources map[string]oauth2.TokenSource
	http    *http.Client
}

// NewOAuthTokenCache builds an empty cache whose token requests time out after TokenRequestTimeout.
func NewOAuthTokenCache() *OAuthTokenCache {
	return &OAuthTokenCache{sources: map[string]oauth2.TokenSource{}, http: &http.Client{Timeout: time.Duration(TokenRequestTimeout)}}
}

// Source returns the cached token source of spec's credentials (tokens are reused until they expire).
func (c *OAuthTokenCache) Source(spec ConnectionSpec) oauth2.TokenSource {
	key := c.key(spec)
	c.mu.Lock()
	defer c.mu.Unlock()
	if src, ok := c.sources[key]; ok {
		return src
	}
	if len(c.sources) >= int(MaxCachedTokenSets) {
		c.sources = map[string]oauth2.TokenSource{}
	}
	cfg := clientcredentials.Config{ClientID: spec.Auth.ClientID, ClientSecret: spec.Secret.ClientSecret, TokenURL: spec.Auth.TokenURL, Scopes: spec.Auth.Scopes}
	src := cfg.TokenSource(context.WithValue(context.Background(), oauth2.HTTPClient, c.http))
	c.sources[key] = src
	return src
}

// key hashes the credential set so secrets are never kept as map keys in clear text.
func (c *OAuthTokenCache) key(spec ConnectionSpec) string {
	parts := []string{spec.Auth.TokenURL, spec.Auth.ClientID, spec.Secret.ClientSecret, strings.Join(spec.Auth.Scopes, string(ScopeSeparator))}
	sum := sha256.Sum256([]byte(strings.Join(parts, string(KeyFieldSeparator))))
	return hex.EncodeToString(sum[:])
}

// OAuthBearerConfigurer is SASL/OAUTHBEARER with an OAuth 2.0 client-credentials token, optionally over TLS.
type OAuthBearerConfigurer struct {
	tls    TLSOptionBuilder
	tokens *OAuthTokenCache
}

// NewOAuthBearerConfigurer builds the configurer over a shared token cache.
func NewOAuthBearerConfigurer(tls TLSOptionBuilder, tokens *OAuthTokenCache) OAuthBearerConfigurer {
	return OAuthBearerConfigurer{tls: tls, tokens: tokens}
}

// Mode implements SecurityConfigurer.
func (OAuthBearerConfigurer) Mode() SecurityMode { return SecuritySASLOAuthBearer }

// Validate implements SecurityConfigurer.
func (c OAuthBearerConfigurer) Validate(spec ConnectionSpec) []FieldError {
	errs := RequiredField{Value: spec.Auth.TokenURL, Path: string(PathTokenURL), Message: MsgTokenURLRequired}.Check()
	if spec.Auth.TokenURL != "" && !EndpointURL(spec.Auth.TokenURL).Valid() {
		errs = append(errs, FieldError{Path: string(PathTokenURL), Code: CodeInvalid, Message: string(MsgTokenURLInvalid)})
	}
	errs = append(errs, RequiredField{Value: spec.Auth.ClientID, Path: string(PathClientID), Message: MsgClientIDRequired}.Check()...)
	errs = append(errs, RequiredField{Value: spec.Secret.ClientSecret, Path: SecretPath(SecretClientSecret), Message: MsgClientSecretRequired}.Check()...)
	if spec.TLS.Enabled {
		errs = append(errs, c.tls.Validate(spec, false)...)
	}
	return errs
}

// Options implements SecurityConfigurer; each SASL session asks the cached source for a valid token.
func (c OAuthBearerConfigurer) Options(spec ConnectionSpec) ([]kgo.Opt, error) {
	source := c.tokens.Source(spec)
	mechanism := oauth.Oauth(func(context.Context) (oauth.Auth, error) {
		tok, err := source.Token()
		if err != nil {
			return oauth.Auth{}, err
		}
		return oauth.Auth{Token: tok.AccessToken}, nil
	})
	return SASLCredentials{tls: c.tls}.Options(spec, mechanism)
}
