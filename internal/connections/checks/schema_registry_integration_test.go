package checks_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// mockRegistry is a Confluent-compatible registry answering GET /subjects for one basic-auth user.
func mockRegistry(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "sr-user" || pass != "sr-pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/subjects" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.schemaregistry.v1+json")
		_, _ = w.Write([]byte(`["orders-value"]`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func registryConnection(t *testing.T, e env, seeds []string, url, password string) connections.Connection {
	t.Helper()
	c := e.saveSecured(t, seeds, connections.Security{Mode: kafka.SecurityPlaintext}, kafka.SecretBundle{RegistrySecret: password})
	c.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: url, Auth: kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBasic, Username: "sr-user"}}
	c.SecretFields = []kafka.SecretField{kafka.SecretRegistry}
	saved, err := e.repo.Update(t.Context(), c, c.Version)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func lastStep(events []connections.StepEvent) connections.StepEvent {
	return events[len(events)-1]
}

func TestSchemaRegistryStepRunsWhenConfigured(t *testing.T) {
	kc := cluster(t)
	reg := mockRegistry(t)
	e := newEnv()
	c := registryConnection(t, e, kc.ListenAddrs(), reg.URL, "sr-pass")
	sink, saved := e.run(t, c.ID, connections.TestOptions{ReadTopic: "orders"})
	step := lastStep(sink.Events)
	if !sink.Result.OK || saved.Health.Status != connections.HealthHealthy || step.Step != connections.StepSchemaRegistry || step.State != connections.StatePassed {
		t.Fatalf("result %+v events %+v", sink.Result, sink.Events)
	}
}

func TestSchemaRegistryDownDegradesHealth(t *testing.T) {
	kc := cluster(t)
	reg := mockRegistry(t)
	e := newEnv()
	c := registryConnection(t, e, kc.ListenAddrs(), reg.URL, "sr-pass")
	reg.Close()
	sink, saved := e.run(t, c.ID, connections.TestOptions{})
	failure := saved.Health.LastFailure
	if sink.Result.OK || saved.Health.Status != connections.HealthDegraded || failure == nil || failure.Step != connections.StepSchemaRegistry {
		t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
	}
	if failure.Reason != kafka.ReasonUnreachable || failure.Message != "Can't reach the schema registry." {
		t.Fatalf("failure %+v", failure)
	}
}

func TestSchemaRegistryWrongPasswordFailsTheRegistryStep(t *testing.T) {
	kc := cluster(t)
	reg := mockRegistry(t)
	e := newEnv()
	c := registryConnection(t, e, kc.ListenAddrs(), reg.URL, "nope")
	sink, saved := e.run(t, c.ID, connections.TestOptions{})
	failure := saved.Health.LastFailure
	if sink.Result.OK || saved.Health.Status != connections.HealthDegraded || failure.Reason != kafka.ReasonAuthFailed {
		t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
	}
}

func TestConnectionWithoutRegistryHasNoRegistryStep(t *testing.T) {
	kc := cluster(t)
	e := newEnv()
	c := e.saveSecured(t, kc.ListenAddrs(), connections.Security{Mode: kafka.SecurityPlaintext}, kafka.SecretBundle{})
	sink, _ := e.run(t, c.ID, connections.TestOptions{})
	for _, ev := range sink.Events {
		if ev.Step == connections.StepSchemaRegistry {
			t.Fatalf("unexpected registry step: %+v", ev)
		}
	}
}
