package specclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/lookups/specclient"
	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/pkg/lookup"
)

type staticToken struct{}

func (staticToken) Token(context.Context) (string, error) { return "w", nil }

func TestRemoteSpecs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/lookup-sources/s1/spec" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(lookup.SourceSpec{ID: "s1", Kind: lookup.KindRedis, Secrets: lookup.SecretBundle{Password: "p"}})
	}))
	defer srv.Close()
	specs := specclient.RemoteSpecs{Fetcher: controlplane.NewClientWith(srv.URL, srv.Client(), staticToken{})}
	spec, err := specs.Spec(context.Background(), "s1")
	if err != nil || spec.Kind != lookup.KindRedis || spec.Secrets.Password != "p" {
		t.Fatalf("spec %+v %v", spec, err)
	}
	if _, err := specs.Spec(context.Background(), "gone"); !errors.Is(err, lookup.ErrSourceNotFound) {
		t.Fatalf("404 must be ErrSourceNotFound: %v", err)
	}
}
