package schemaregistry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/pkg/kafka"
)

func TestPingSignsRequestsPerAuthMode(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		if r.URL.Path != string(PathSubjects) {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := NewClient(nil, DefaultAuthRegistry())
	spec := kafka.ConnectionSpec{SchemaRegistry: &kafka.SchemaRegistrySpec{URL: srv.URL + "/", Auth: kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBearer}},
		Secret: kafka.SecretBundle{RegistrySecret: "tok"}}
	if err := client.Ping(context.Background(), spec); err != nil || got != "Bearer tok" {
		t.Fatalf("bearer: %v %q", err, got)
	}
	spec.SchemaRegistry.Auth = kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBasic, Username: "u"}
	if err := client.Ping(context.Background(), spec); err != nil || got == "" || got == "Bearer tok" {
		t.Fatalf("basic: %v %q", err, got)
	}
	if err := client.Ping(context.Background(), kafka.ConnectionSpec{}); err != ErrNotConfigured {
		t.Fatalf("not configured: %v", err)
	}
}

func TestStatusReasons(t *testing.T) {
	cases := map[int]kafka.FailureReason{http.StatusUnauthorized: kafka.ReasonAuthFailed, http.StatusForbidden: kafka.ReasonNotAuthorized, http.StatusTeapot: kafka.ReasonUnknown}
	for code, want := range cases {
		if got, ok := DefaultStatusReasons.ReasonOf(fmt.Errorf("x: %w", &StatusError{Code: code})); !ok || got != want {
			t.Fatalf("%d: %s", code, got)
		}
	}
	if _, ok := DefaultStatusReasons.ReasonOf(fmt.Errorf("dial")); ok {
		t.Fatal("non-status errors are classified elsewhere")
	}
}
