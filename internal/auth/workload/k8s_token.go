package workload

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/problem"
)

// K8sConst names TokenReview API constants.
type K8sConst string

const (
	TokenReviewPath       K8sConst = "/apis/authentication.k8s.io/v1/tokenreviews"
	TokenReviewAPIVersion K8sConst = "authentication.k8s.io/v1"
	TokenReviewKind       K8sConst = "TokenReview"
	ServiceAccountToken   K8sConst = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// ReviewTimeout bounds one TokenReview call.
type ReviewTimeout time.Duration

const DefaultReviewTimeout ReviewTimeout = ReviewTimeout(5 * time.Second)

// K8sTokenReviewAuthenticator verifies projected ServiceAccount tokens through the TokenReview API.
type K8sTokenReviewAuthenticator struct {
	cfg    config.K8sReviewConfig
	client *http.Client
}

// NewK8sTokenReviewAuthenticator builds the authenticator, trusting the cluster CA when present.
func NewK8sTokenReviewAuthenticator(cfg config.K8sReviewConfig) (*K8sTokenReviewAuthenticator, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if pem, err := os.ReadFile(cfg.CAFile); err == nil {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		tlsCfg.RootCAs = pool
	}
	client := &http.Client{Timeout: time.Duration(DefaultReviewTimeout), Transport: &http.Transport{TLSClientConfig: tlsCfg}}
	return &K8sTokenReviewAuthenticator{cfg: cfg, client: client}, nil
}

// Kind implements Authenticator.
func (a *K8sTokenReviewAuthenticator) Kind() Kind {
	return config.WorkloadK8sToken
}

// Authenticate submits the bearer token for review with the configured audience.
func (a *K8sTokenReviewAuthenticator) Authenticate(r *http.Request) (auth.Principal, error) {
	token := BearerToken(r)
	if token == "" {
		return auth.Anonymous, ErrUnauthenticated
	}
	review, err := a.review(r, token)
	if err != nil {
		return auth.Anonymous, problem.NewError(problem.KindUnavailable, "Workload identity check unavailable. Retry shortly.").Wrap(err)
	}
	if !review.Status.Authenticated {
		return auth.Anonymous, ErrUnauthenticated
	}
	return auth.Principal{Kind: auth.PrincipalWorkload, WorkloadName: review.Status.User.Username}, nil
}

func (a *K8sTokenReviewAuthenticator) review(r *http.Request, token string) (tokenReview, error) {
	body, _ := json.Marshal(tokenReview{APIVersion: string(TokenReviewAPIVersion), Kind: string(TokenReviewKind),
		Spec: reviewSpec{Token: token, Audiences: []string{a.cfg.Audience}}})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, a.cfg.APIServerURL+string(TokenReviewPath), bytes.NewReader(body))
	if err != nil {
		return tokenReview{}, err
	}
	req.Header.Set(string(HeaderAuthorization), string(SchemeBearer)+a.reviewerToken())
	req.Header.Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	resp, err := a.client.Do(req)
	if err != nil {
		return tokenReview{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return tokenReview{}, fmt.Errorf("workload: tokenreview status %d", resp.StatusCode)
	}
	var out tokenReview
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (a *K8sTokenReviewAuthenticator) reviewerToken() string {
	if a.cfg.ReviewerToken != "" {
		return a.cfg.ReviewerToken
	}
	raw, _ := os.ReadFile(string(ServiceAccountToken))
	return string(bytes.TrimSpace(raw))
}

type tokenReview struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Spec       reviewSpec   `json:"spec"`
	Status     reviewStatus `json:"status"`
}

type reviewSpec struct {
	Token     string   `json:"token"`
	Audiences []string `json:"audiences"`
}

type reviewStatus struct {
	Authenticated bool `json:"authenticated"`
	User          struct {
		Username string `json:"username"`
	} `json:"user"`
}
