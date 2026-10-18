package checks_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// tokenEndpoint is a mock OAuth client-credentials endpoint issuing one fixed token per known client.
type tokenEndpoint struct {
	srv     *httptest.Server
	hits    atomic.Int32
	clients map[string]string
	tokens  map[string]string
}

func newTokenEndpoint(t *testing.T) *tokenEndpoint {
	e := &tokenEndpoint{
		clients: map[string]string{"router": "s3cret", "stranger": "pw"},
		tokens:  map[string]string{"router": "good-token", "stranger": "unknown-token"},
	}
	e.srv = httptest.NewServer(http.HandlerFunc(e.serve))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *tokenEndpoint) serve(w http.ResponseWriter, r *http.Request) {
	e.hits.Add(1)
	id, secret, ok := r.BasicAuth()
	if !ok {
		_ = r.ParseForm()
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	w.Header().Set("Content-Type", "application/json")
	if want, known := e.clients[id]; !known || want != secret {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": e.tokens[id], "token_type": "bearer", "expires_in": 3600})
}

// oauthCluster is a kfake cluster whose SASL handshake only accepts OAUTHBEARER with acceptedToken.
func oauthCluster(t *testing.T, acceptedToken string) *kfake.Cluster {
	kc := cluster(t)
	kc.ControlKey(int16(kmsg.SASLHandshake), func(req kmsg.Request) (kmsg.Response, error, bool) {
		kc.KeepControl()
		hs := req.(*kmsg.SASLHandshakeRequest)
		resp := hs.ResponseKind().(*kmsg.SASLHandshakeResponse)
		resp.SupportedMechanisms = []string{"OAUTHBEARER"}
		if hs.Mechanism != "OAUTHBEARER" {
			resp.ErrorCode = kerr.UnsupportedSaslMechanism.Code
		}
		return resp, nil, true
	})
	kc.ControlKey(int16(kmsg.SASLAuthenticate), func(req kmsg.Request) (kmsg.Response, error, bool) {
		kc.KeepControl()
		auth := req.(*kmsg.SASLAuthenticateRequest)
		resp := auth.ResponseKind().(*kmsg.SASLAuthenticateResponse)
		if !bytes.Contains(auth.SASLAuthBytes, []byte("auth=Bearer "+acceptedToken+"\x01")) {
			resp.ErrorCode = kerr.SaslAuthenticationFailed.Code
		}
		return resp, nil, true
	})
	return kc
}

func oauthSecurity(tokenURL, clientID string) connections.Security {
	return connections.Security{Mode: kafka.SecuritySASLOAuthBearer, Auth: kafka.AuthSpec{TokenURL: tokenURL, ClientID: clientID, Scopes: []string{"kafka"}}}
}

func TestOAuthBearerPassesFullChecklistAndCachesTheToken(t *testing.T) {
	idp := newTokenEndpoint(t)
	kc := oauthCluster(t, "good-token")
	e := newEnv()
	c := e.saveSecured(t, kc.ListenAddrs(), oauthSecurity(idp.srv.URL, "router"), kafka.SecretBundle{ClientSecret: "s3cret"})
	for range 2 {
		sink, saved := e.run(t, c.ID, connections.TestOptions{ReadTopic: "orders", WriteTopic: "orders"})
		if !sink.Result.OK || saved.Health.Status != connections.HealthHealthy {
			t.Fatalf("result %+v events %+v", sink.Result, sink.Events)
		}
	}
	if hits := idp.hits.Load(); hits != 1 {
		t.Fatalf("token endpoint hit %d times, want 1 (cached token)", hits)
	}
}

func TestOAuthBearerWrongClientSecretFailsAuthentication(t *testing.T) {
	idp := newTokenEndpoint(t)
	kc := oauthCluster(t, "good-token")
	e := newEnv()
	c := e.saveSecured(t, kc.ListenAddrs(), oauthSecurity(idp.srv.URL, "router"), kafka.SecretBundle{ClientSecret: "wrong"})
	sink, saved := e.run(t, c.ID, connections.TestOptions{})
	failure := saved.Health.LastFailure
	if sink.Result.OK || saved.Health.Status != connections.HealthAuthFailed || failure == nil || failure.Step != connections.StepAuthenticated {
		t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
	}
	if failure.Reason != kafka.ReasonAuthFailed {
		t.Fatalf("reason %q", failure.Reason)
	}
}

func TestOAuthBearerTokenRejectedByBrokerFailsAuthentication(t *testing.T) {
	idp := newTokenEndpoint(t)
	kc := oauthCluster(t, "good-token")
	e := newEnv()
	c := e.saveSecured(t, kc.ListenAddrs(), oauthSecurity(idp.srv.URL, "stranger"), kafka.SecretBundle{ClientSecret: "pw"})
	sink, saved := e.run(t, c.ID, connections.TestOptions{})
	if sink.Result.OK || saved.Health.Status != connections.HealthAuthFailed || saved.Health.LastFailure.Reason != kafka.ReasonAuthFailed {
		t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
	}
}
