package connections_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/kafka"
)

func validDraft() connections.Draft {
	return connections.Draft{
		Name: "orders-prod", EnvTag: connections.EnvProd, BootstrapServers: []string{"kafka-1:9092", "kafka-2:9092"},
		Security: connections.Security{Mode: kafka.SecuritySASLScram512, Auth: kafka.AuthSpec{Username: "svc"}},
		Secrets:  kafka.SecretBundle{Password: "pw"}, ProbeEnabled: true,
	}
}

func fieldPaths(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var verr *api.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("not a validation error: %v", err)
	}
	var out []string
	for _, fe := range verr.Errors {
		out = append(out, fe.Path)
	}
	return out
}

func TestValidatorRules(t *testing.T) {
	v := connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())
	mutate := func(f func(*connections.Draft)) connections.Draft { d := validDraft(); f(&d); return d }
	cases := []struct {
		name  string
		draft connections.Draft
		first string
		count int
	}{
		{"valid", validDraft(), "", 0},
		{"missing name", mutate(func(d *connections.Draft) { d.Name = " " }), "/name", 1},
		{"bad name", mutate(func(d *connections.Draft) { d.Name = "a/b" }), "/name", 1},
		{"bad env", mutate(func(d *connections.Draft) { d.EnvTag = "qa" }), "/envTag", 1},
		{"demo kind rejected", mutate(func(d *connections.Draft) { d.Kind = kafka.ClusterDemo }), "/kind", 1},
		{"no brokers", mutate(func(d *connections.Draft) { d.BootstrapServers = nil }), "/bootstrapServers", 1},
		{"bad broker index", mutate(func(d *connections.Draft) { d.BootstrapServers = []string{"a:1", "nope", "b:99999"} }), "/bootstrapServers/1", 2},
		{"duplicate broker", mutate(func(d *connections.Draft) { d.BootstrapServers = []string{"a:1", "a:1"} }), "/bootstrapServers/1", 1},
		{"ipv6 broker ok", mutate(func(d *connections.Draft) { d.BootstrapServers = []string{"[::1]:9092"} }), "", 0},
		{"sasl username", mutate(func(d *connections.Draft) { d.Security.Auth.Username = "" }), "/security/username", 1},
		{"sasl password", mutate(func(d *connections.Draft) { d.Secrets = kafka.SecretBundle{} }), "/secrets/password", 1},
		{"unknown property", mutate(func(d *connections.Draft) { d.Properties = map[kafka.PropertyKey]string{"acks": "all"} }), "/properties/acks", 1},
		{"registry ok", mutate(func(d *connections.Draft) { d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "https://sr:8081"} }), "", 0},
		{"registry bad url", mutate(func(d *connections.Draft) { d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "sr:8081"} }), "/schemaRegistry/url", 1},
		{"registry basic needs both", mutate(func(d *connections.Draft) {
			d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "http://sr", Auth: kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBasic}}
		}), "/schemaRegistry/auth/username", 2},
		{"registry bearer token", mutate(func(d *connections.Draft) {
			d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "http://sr", Auth: kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBearer}}
		}), "/secrets/schemaRegistrySecret", 1},
		{"registry bad mode", mutate(func(d *connections.Draft) {
			d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "http://sr", Auth: kafka.RegistryAuthSpec{Mode: "kerberos"}}
		}), "/schemaRegistry/auth/mode", 1},
		{"oauth fields", mutate(func(d *connections.Draft) { d.Security = connections.Security{Mode: kafka.SecuritySASLOAuthBearer} }), "/security/tokenUrl", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldPaths(t, v.Validate(context.Background(), tc.draft))
			if len(got) != tc.count || (tc.count > 0 && got[0] != tc.first) {
				t.Fatalf("got %v, want first %q count %d", got, tc.first, tc.count)
			}
		})
	}
	many := validDraft()
	for i := 0; i < 51; i++ {
		many.BootstrapServers = append(many.BootstrapServers, "h:1")
	}
	if got := fieldPaths(t, v.Validate(context.Background(), many)); len(got) != 1 || got[0] != "/bootstrapServers" {
		t.Fatalf("too many seeds: %v", got)
	}
}

func TestHealthMapperAndHints(t *testing.T) {
	m := connections.DefaultHealthMapper()
	cases := map[connections.TestStep]connections.HealthStatus{
		connections.StepBrokersReachable: connections.HealthUnreachable,
		connections.StepAuthenticated:    connections.HealthAuthFailed,
		connections.StepListTopics:       connections.HealthDegraded,
		connections.StepCanWrite:         connections.HealthDegraded,
	}
	for step, want := range cases {
		if got := m.Map(&connections.LastFailure{Step: step}); got != want {
			t.Fatalf("%s: got %s want %s", step, got, want)
		}
	}
	if m.Map(nil) != connections.HealthHealthy {
		t.Fatal("no failure must be healthy")
	}
	hints := connections.DefaultHintCatalog()
	if h := hints.Lookup(connections.StepCanWrite, kafka.ReasonNotAuthorized); h.Message != "Can't write to the test topic." {
		t.Fatalf("step override: %+v", h)
	}
	if h := hints.Lookup(connections.StepAuthenticated, kafka.ReasonSASLMismatch); h.Hint == "" {
		t.Fatal("reason default missing")
	}
	if h := hints.Lookup(connections.StepAuthenticated, "weird"); h.Message != "The check failed." {
		t.Fatalf("unknown fallback: %+v", h)
	}
}

func TestRateMath(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	next, rates := connections.RateMath{}.Next(nil, now, map[string]int64{"a": 10})
	if next == nil || len(rates) != 0 {
		t.Fatal("first snapshot must store and show no rates")
	}
	again, rates := connections.RateMath{}.Next(next, now.Add(2*time.Second), map[string]int64{"a": 20})
	if again != nil || len(rates) != 0 {
		t.Fatal("too-recent snapshot must keep previous rates")
	}
	later, rates := connections.RateMath{}.Next(next, now.Add(10*time.Second), map[string]int64{"a": 110, "b": 5})
	if later == nil || rates["a"] == nil || *rates["a"] != 10 || rates["b"] != nil {
		t.Fatalf("rate: %+v", rates)
	}
	_, rates = connections.RateMath{}.Next(later, now.Add(20*time.Second), map[string]int64{"a": 50})
	if rates["a"] != nil {
		t.Fatal("offset regression (recreated topic) must not show a negative rate")
	}
}

func TestTopicRequestValidator(t *testing.T) {
	v := connections.TopicRequestValidator{}
	spec, err := v.Spec(connections.TopicRequest{Name: "orders.v1"})
	if err != nil || spec.Partitions != 6 || spec.ReplicationFactor != -1 || spec.RetentionMs != 7*24*3600*1000 {
		t.Fatalf("defaults: %+v %v", spec, err)
	}
	p, rf, ret := int32(0), int16(0), int64(-5)
	_, err = v.Spec(connections.TopicRequest{Name: "..", Partitions: &p, ReplicationFactor: &rf, RetentionMs: &ret})
	if got := fieldPaths(t, err); len(got) != 4 || got[0] != "/name" {
		t.Fatalf("errors: %v", got)
	}
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'a'
	}
	if got := fieldPaths(t, func() error { _, e := v.Spec(connections.TopicRequest{Name: string(long)}); return e }()); len(got) != 1 {
		t.Fatalf("long name: %v", got)
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{"ROUTER_CONNECTION_PROBE_INTERVAL": "30s", "ROUTER_CONNECTION_TEST_STEPS": "brokers_reachable, authenticated", "ROUTER_CONNECTION_PROBE_ENABLED": "false"}
	s, err := connections.SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}.Load()
	if err != nil || s.ProbeInterval != 30*time.Second || len(s.TestPlan) != 2 || s.ProbeEnabled {
		t.Fatalf("settings: %+v %v", s, err)
	}
	env["ROUTER_CONNECTION_PROBE_INTERVAL"] = "soon"
	if _, err := (connections.SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}).Load(); err == nil {
		t.Fatal("bad duration must fail")
	}
}
