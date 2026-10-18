// Package dlqtest provides fakes for dead-letter tests: an in-memory route source, a kfake
// cluster with DLQ records written through the record contract, and a fully assembled Manager.
package dlqtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// Fixture names used by the fakes.
const (
	RouteID     = "11111111-1111-1111-1111-111111111111"
	TeamID      = "22222222-2222-2222-2222-222222222222"
	DLQTopic    = "router.dlq.orders"
	DestTopic   = "orders-public"
	Connection  = "c1"
	RouteConfig = `{"apiVersion":"router/v1","name":"orders","source":{"connectionId":"c1","topics":["orders"],"format":{"type":"json"}},
"branches":[{"id":"main","filter":{"type":"cond","field":"value.amount","op":"gt","value":100},
"destination":{"connectionId":"c1","topic":"orders-public","format":{"type":"json"}}}],"errorPolicy":{"onError":"dlq","dlqTopic":"router.dlq.orders"}}`
)

// BaseTime is the failure time of record 0.
var BaseTime = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

// Routes is an in-memory RouteSource with one deployed route.
type Routes struct {
	Info    dlq.RouteInfo
	Configs []dlq.VersionConfig
}

// NewRoutes returns the fixture route with version 1 active.
func NewRoutes() Routes {
	return Routes{
		Info:    dlq.RouteInfo{ID: RouteID, Name: "orders", TeamID: TeamID, ActiveVersionID: "v1"},
		Configs: []dlq.VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(RouteConfig)}},
	}
}

func (f Routes) Route(_ context.Context, id string) (dlq.RouteInfo, error) {
	if id != f.Info.ID {
		return dlq.RouteInfo{}, dlq.ErrRouteNotFound
	}
	return f.Info, nil
}

func (f Routes) Routes(context.Context) ([]dlq.RouteInfo, error) { return []dlq.RouteInfo{f.Info}, nil }

func (f Routes) Versions(context.Context, string) ([]dlq.VersionConfig, error) { return f.Configs, nil }

func (f Routes) Version(_ context.Context, id string) (dlq.VersionConfig, error) {
	for _, v := range f.Configs {
		if v.ID == id {
			return v, nil
		}
	}
	return dlq.VersionConfig{}, errors.New("dlqtest: no version")
}

// Cluster starts a kfake cluster with the DLQ and destination topics.
func Cluster(t *testing.T) dlq.SeedClientOpener {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, DLQTopic, DestTopic))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return dlq.SeedClientOpener{Seeds: c.ListenAddrs()}
}

// Produce writes one DLQ record per value (failure i minutes after BaseTime).
func Produce(t *testing.T, opener dlq.SeedClientOpener, values ...string) {
	t.Helper()
	cl, err := opener.Open(context.Background(), Connection, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for i, v := range values {
		raw := record.Codec{}.Encode(record.Record{Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte(v),
			Failure: record.Failure{RouteID: RouteID, VersionNo: 1, Branch: "main", Step: "s", ErrorType: record.ErrorStep, Message: "m", FailedAt: BaseTime.Add(time.Duration(i) * time.Minute)},
			Source:  record.SourcePosition{Topic: "orders", Offset: int64(i)}})
		hs := []kgo.RecordHeader{}
		for _, h := range raw.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		if err := cl.ProduceSync(context.Background(), &kgo.Record{Topic: DLQTopic, Key: raw.Key, Value: raw.Value, Headers: hs}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

// Manager assembles a Manager over the fakes with in-memory state.
func Manager(t *testing.T, opener dlq.SeedClientOpener) *dlq.Manager {
	t.Helper()
	return ManagerWith(t, opener, func(*dlq.AssemblyDeps) {})
}

// ManagerWith assembles a Manager after tune adjusted the deps (masking, registries, executors).
func ManagerWith(t *testing.T, opener dlq.SeedClientOpener, tune func(*dlq.AssemblyDeps)) *dlq.Manager {
	t.Helper()
	deps := dlq.AssemblyDeps{
		Settings: dlq.Settings{StateBackend: dlq.StateBackendMemory, Executor: dlq.ExecutorInProcess, Scripts: dlq.ScriptsNone},
		Routes:   NewRoutes(), Clients: opener, Compiler: compile.NewDefaultCompiler(), Scripts: dlq.NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: dlq.ScopeAuditTrail{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	tune(&deps)
	m, err := dlq.NewAssembly(deps).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m
}
