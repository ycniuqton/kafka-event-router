package cpclient_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/controlapi"
	"github.com/eventrouter/router/internal/runtime/cpclient"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/runtime/state/statetest"
)

type noLifecycle struct{}

func (noLifecycle) MarkActive(context.Context, string) error                 { return nil }
func (noLifecycle) MarkFailed(context.Context, string, string) (bool, error) { return false, nil }

type noPause struct{}

func (noPause) Pause(context.Context, string) error { return nil }

func server(t *testing.T) (*statetest.Memory, *changes.Broker, *cpclient.Client) {
	t.Helper()
	mem := statetest.New()
	broker := changes.NewBroker()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	stores := mem.Stores()
	agg := &controlapi.RuntimeStatusAggregator{Assignments: stores.Assignments, Runtime: stores.Runtime}
	mod := controlapi.NewInternalModule(controlapi.InternalDeps{
		Workers: &controlapi.WorkerAPI{Stores: stores, Aggregator: agg, Timings: domain.DefaultTimings(), Now: time.Now},
		Halts:   &controlapi.HaltService{Stores: stores, Lifecycle: noLifecycle{}, Pauser: noPause{}, Log: log, Now: time.Now},
		Changes: broker, Responder: responder,
	})
	r := chi.NewRouter()
	api.NewMounter(responder, nil, nil).Mount(r, mod)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	cp := controlplane.NewClientWith(srv.URL, srv.Client(), controlplane.StaticCredential("token"))
	return mem, broker, cpclient.New(cp, "w1")
}

func TestClientRoundTripsTheWorkerContract(t *testing.T) {
	mem, _, c := server(t)
	ctx := context.Background()
	mem.Demands["r1"] = state.RouteDemand{RouteID: "r1", ActiveVersionID: "v1"}
	mem.Configs["v1"] = []byte(`{"name":"orders"}`)
	mem.VersionNos["v1"] = 1
	mem.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w1", TargetVersionID: "v1"})
	reply, err := c.Heartbeat(ctx, domain.Heartbeat{Host: "h", Zone: "z", Capacity: 5})
	if err != nil || reply.AssignmentsETag == "" {
		t.Fatalf("heartbeat %+v %v", reply, err)
	}
	set, notModified, err := c.Assignments(ctx, "")
	if err != nil || notModified || len(set.Assignments) != 1 || set.ETag != reply.AssignmentsETag {
		t.Fatalf("assignments %+v %v", set, err)
	}
	if _, notModified, _ := c.Assignments(ctx, set.ETag); !notModified {
		t.Fatal("expected 304 for an unchanged set")
	}
	doc, err := c.VersionConfig(ctx, "v1")
	if err != nil || string(doc) != `{"name":"orders"}` {
		t.Fatalf("config %s %v", doc, err)
	}
	if err := c.ReportStatus(ctx, []domain.ReplicaStatus{{RouteID: "r1", VersionID: "v1", State: domain.RunnerRunning}}); err != nil {
		t.Fatal(err)
	}
	if mem.Runtime["r1"].Observed != domain.ObservedRunning {
		t.Fatalf("runtime %+v", mem.Runtime["r1"])
	}
	if err := c.ReportHalt(ctx, domain.HaltReport{RouteID: "r1", VersionID: "v1", Reason: domain.HaltSourceFatal, Message: "denied"}); err != nil {
		t.Fatal(err)
	}
	if mem.Runtime["r1"].HaltedAt == nil {
		t.Fatal("halt not recorded")
	}
}

func TestClientReceivesChangeHints(t *testing.T) {
	_, broker, c := server(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hints, err := c.Changes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		broker.Publish(domain.ChangeHint{RouteID: "r9", Kind: domain.ChangeRoute})
		select {
		case h := <-hints:
			if h.RouteID != "r9" || h.Kind != domain.ChangeRoute {
				t.Fatalf("hint %+v", h)
			}
			return
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Fatal("no hint over SSE")
		}
	}
}
