package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

type captureTrail struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (c *captureTrail) Record(_ context.Context, e AuditEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

type fixedGate struct{ decision GateDecision }

func (g fixedGate) Check(context.Context, RetryTarget) (GateDecision, error) { return g.decision, nil }

func workflowManager(t *testing.T, opener SeedClientOpener, gate ReplayGate, trail AuditTrail) *Manager {
	t.Helper()
	routes := fakeRoutes{
		route:    RouteInfo{ID: "r1", Name: "orders", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(e2eRouteConfig)}},
	}
	m, err := NewAssembly(AssemblyDeps{
		Settings: Settings{StateBackend: StateBackendMemory, Executor: ExecutorInProcess, Scripts: ScriptsNone},
		Routes:   routes, Clients: opener, Compiler: compile.NewDefaultCompiler(), Scripts: NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: trail, Gate: gate, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWorkflowGroupsEditDryRunAndReplay(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	produceValues(t, opener, []string{`{"amount":200,"cust_id":1}`, `{"amount":50}`, `{bad`})
	trail := &captureTrail{}
	m, ctx := workflowManager(t, opener, nil, trail), context.Background()
	groups, err := m.Groups(ctx, "r1", Query{})
	if err != nil || len(groups.Items) != 1 || groups.Items[0].Count != 3 || groups.ByClass[record.ClassData] != 3 || groups.Approximate {
		t.Fatalf("groups %v %+v", err, groups)
	}
	list, _ := m.List(ctx, "r1", Query{Fingerprint: groups.Items[0].Fingerprint})
	if len(list.Items) != 3 || list.Items[0].ErrorClass != record.ClassData {
		t.Fatalf("fingerprint list %+v", list.Items)
	}
	bad := list.Items[0].ID
	edit := &RecordEdit{Value: &EncodedValue{Encoding: EncodingJSON, Data: map[string]any{"amount": 500, "cust_id": 9}}}
	dry, err := m.Replay(ctx, Viewer{}, Actor{UserID: "u1"}, "r1", bad, ReplayRequest{Edit: edit, DryRun: true})
	if err != nil || dry.Dry == nil || dry.Dry.Outcome != DryWouldSend || len(dry.Dry.Outputs) != 1 || dry.Dry.Outputs[0].Topic != "orders-public" {
		t.Fatalf("dry %v %+v", err, dry.Dry)
	}
	assertDestination(t, opener, 0)
	done, err := m.Replay(ctx, Viewer{}, Actor{UserID: "u1"}, "r1", bad, ReplayRequest{Edit: edit, Note: "fixed JSON"})
	if err != nil || done.Status != ReplayDone || done.Report.Counts()[OutcomeSent] != 1 || len(done.Diff) == 0 {
		t.Fatalf("replay %v %+v", err, done)
	}
	assertDestination(t, opener, 1)
	last := trail.events[len(trail.events)-1]
	detail, _ := last.Detail.(EditAuditDetail)
	if last.Action != ActionDLQEditedReplayed || detail.Note != "fixed JSON" || detail.ID != bad || len(detail.Diff) == 0 {
		t.Fatalf("audit %+v", last)
	}
	report, err := m.DryRun(ctx, Viewer{}, "r1", Selection{Match: &Query{}})
	if err != nil || report.Sampled != 2 || report.Counts[DryWouldSend] != 1 || report.Counts[DryFiltered] != 1 {
		t.Fatalf("dry-run selection %v %+v", err, report)
	}
}

func TestWorkflowFourEyesGate(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	produceValues(t, opener, []string{`{"amount":200,"cust_id":1}`})
	m, ctx := workflowManager(t, opener, fixedGate{decision: GateRequireApproval}, NoAuditTrail{}), context.Background()
	list, _ := m.List(ctx, "r1", Query{})
	id := list.Items[0].ID
	if _, err := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r1", Selection{IDs: []string{id}}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("sync retry must need approval: %v", err)
	}
	edit := &RecordEdit{Value: &EncodedValue{Encoding: EncodingJSON, Data: map[string]any{"amount": 900, "cust_id": 3}}}
	res, err := m.Replay(ctx, Viewer{}, Actor{UserID: "u1"}, "r1", id, ReplayRequest{Edit: edit})
	if err != nil || res.Status != ReplayPendingApproval || res.Pending == nil || !res.Pending.Edited || res.Pending.Edit == nil {
		t.Fatalf("pending %v %+v", err, res)
	}
	assertDestination(t, opener, 0)
	report, err := m.ReplayApproved(ctx, Actor{UserID: "u1"}, "job1", "r1", id, res.Pending.Edit)
	if err != nil || report.Counts()[OutcomeSent] != 1 {
		t.Fatalf("approved %v %+v", err, report)
	}
	assertDestination(t, opener, 1)
	deny := workflowManager(t, opener, fixedGate{decision: GateDeny}, NoAuditTrail{})
	if _, err := deny.Replay(ctx, Viewer{}, Actor{}, "r1", id, ReplayRequest{}); !errors.Is(err, ErrReplayDenied) {
		t.Fatalf("deny %v", err)
	}
}
