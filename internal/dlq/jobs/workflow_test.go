package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
)

type gateOf struct{ decision dlq.GateDecision }

func (g gateOf) CheckReplay(context.Context, dlq.RouteInfo) (dlq.GateDecision, error) {
	return g.decision, nil
}

// fakePages records how the runner pages a job.
type fakePages struct {
	sizes    []int
	ids      [][]string
	replayed []string
}

func (p *fakePages) ActPageSized(_ context.Context, req dlq.PageRequest, _ dlq.Query) (dlq.JobPage, error) {
	p.sizes = append(p.sizes, req.Size)
	return dlq.JobPage{Report: dlq.ActionReport{Results: make([]dlq.ItemResult, 1)}, Done: true}, nil
}

func (p *fakePages) ActIDsPage(_ context.Context, req dlq.PageRequest, ids []string) (dlq.JobPage, error) {
	start := 0
	if req.Cursor != "" {
		start = len(p.ids) * req.Size
	}
	end := min(len(ids), start+req.Size)
	p.ids = append(p.ids, ids[start:end])
	res := make([]dlq.ItemResult, end-start)
	for i := range res {
		res[i].Outcome = dlq.OutcomeSent
	}
	return dlq.JobPage{Report: dlq.ActionReport{Results: res}, Cursor: "next", Done: end >= len(ids)}, nil
}

func (p *fakePages) ReplayApproved(_ context.Context, _ dlq.Actor, _, _, id string, edit *dlq.RecordEdit) (dlq.ActionReport, error) {
	if edit == nil {
		return dlq.ActionReport{}, errors.New("edit lost")
	}
	p.replayed = append(p.replayed, id)
	return dlq.ActionReport{Results: []dlq.ItemResult{{Outcome: dlq.OutcomeSent}}}, nil
}

func (p *fakePages) CountMatches(context.Context, string, dlq.Query) (int, error) { return 7, nil }

func runner(repo jobs.Repository, pages jobs.Pages) *jobs.Runner {
	return &jobs.Runner{Repo: repo, Pages: pages, Notifier: jobs.Notifiers{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now}
}

func TestPacerBoundsRate(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	p := jobs.Pacer{Rate: 50, Now: func() time.Time { return now }}
	if p.Size() != 100 {
		t.Fatalf("size %d", p.Size())
	}
	if d := p.Delay(100, now); d != 2*time.Second {
		t.Fatalf("delay %v", d)
	}
	if d := p.Delay(100, now.Add(-3*time.Second)); d != 0 {
		t.Fatalf("late page must not wait: %v", d)
	}
	if (jobs.Pacer{Rate: 1000}).Size() != int(dlq.MaxSyncAction) || (jobs.Pacer{Rate: 0}).Size() != 100 {
		t.Fatal("size bounds")
	}
	free := jobs.PacerOf{}.For(jobs.Job{Action: dlq.ActionDiscard, Rate: 1}, time.Now)
	if !free.Unlimited || free.Size() != int(dlq.MaxSyncAction) || free.Delay(500, now) != 0 {
		t.Fatal("discard jobs are not throttled")
	}
}

func TestServiceGatesReplayJobs(t *testing.T) {
	ctx, route := context.Background(), dlq.RouteInfo{ID: "r1", Name: "orders"}
	trail := &recordingTrail{}
	svc := jobs.Service{Repo: jobs.NewMemoryRepository(time.Now), Preparer: fixedPreparer{route: route}, Audit: trail,
		Gate: gateOf{decision: dlq.GateRequireApproval}}
	j, err := svc.CreateWith(ctx, dlq.ActionRetry, dlq.Actor{UserID: "u1"}, "r1", jobs.CreateRequest{IDs: []string{"a", "b"}, Rate: 10, Note: "fix"})
	if err != nil || j.Status != jobs.StatusPendingApproval || !j.Approval.Required || j.Rate != 10 || j.Note != "fix" {
		t.Fatalf("pending job %v %+v", err, j)
	}
	if trail.events[0].Action != dlq.ActionDLQReplayRequest {
		t.Fatalf("audit %+v", trail.events)
	}
	disc, _ := svc.CreateWith(ctx, dlq.ActionDiscard, dlq.Actor{UserID: "u1"}, "r1", jobs.CreateRequest{})
	if disc.Status != jobs.StatusQueued || disc.Rate != int(jobs.DefaultRate) {
		t.Fatalf("discard never waits: %+v", disc)
	}
	svc.Gate = gateOf{decision: dlq.GateDeny}
	if _, err := svc.CreateWith(ctx, dlq.ActionRetry, dlq.Actor{}, "r1", jobs.CreateRequest{}); !errors.Is(err, dlq.ErrReplayDenied) {
		t.Fatalf("deny %v", err)
	}
}

func TestServiceDecideIsFourEyes(t *testing.T) {
	ctx, route := context.Background(), dlq.RouteInfo{ID: "r1"}
	repo := jobs.NewMemoryRepository(time.Now)
	svc := jobs.Service{Repo: repo, Preparer: fixedPreparer{route: route}, Gate: gateOf{decision: dlq.GateRequireApproval}}
	j, _ := svc.CreateWith(ctx, dlq.ActionRetry, dlq.Actor{UserID: "u1"}, "r1", jobs.CreateRequest{IDs: []string{"a"}})
	if _, err := svc.Decide(ctx, j.ID, jobs.Decision{Approve: true, By: "u1"}); !errors.Is(err, dlq.ErrSelfApproval) {
		t.Fatalf("self approval %v", err)
	}
	if pending, _ := svc.Pending(ctx); len(pending) != 1 {
		t.Fatalf("pending %d", len(pending))
	}
	ok, err := svc.Decide(ctx, j.ID, jobs.Decision{Approve: true, By: "u2", Note: "looks right"})
	if err != nil || ok.Status != jobs.StatusQueued || ok.Approval.DecidedBy != "u2" || ok.Approval.DecisionNote != "looks right" {
		t.Fatalf("approve %v %+v", err, ok)
	}
	if _, err := svc.Decide(ctx, j.ID, jobs.Decision{By: "u3"}); !errors.Is(err, dlq.ErrJobNotPending) {
		t.Fatalf("second decision %v", err)
	}
	r, _ := svc.CreateWith(ctx, dlq.ActionRetry, dlq.Actor{UserID: "u1"}, "r1", jobs.CreateRequest{IDs: []string{"b"}})
	rejected, _ := svc.Decide(ctx, r.ID, jobs.Decision{By: "u2"})
	if rejected.Status != jobs.StatusRejected || !rejected.Status.Terminal() || rejected.FinishedAt == nil {
		t.Fatalf("reject %+v", rejected)
	}
}

func TestRunnerPagesIDsAndEditedReplays(t *testing.T) {
	ctx := context.Background()
	repo := jobs.NewMemoryRepository(time.Now)
	pages := &fakePages{}
	ids := []string{"a", "b", "c"}
	if _, err := repo.Create(ctx, jobs.NewJob{RouteID: "r1", Action: dlq.ActionRetry, Match: jobs.Match{IDs: ids}, Rate: 1000}); err != nil {
		t.Fatal(err)
	}
	if !runner(repo, pages).Tick(ctx) || len(pages.ids) != 1 || len(pages.ids[0]) != 3 {
		t.Fatalf("id pages %+v", pages.ids)
	}
	edit := &dlq.RecordEdit{Value: &dlq.EncodedValue{Encoding: dlq.EncodingJSON, Data: map[string]any{"a": 1}}}
	pending, _ := repo.Create(ctx, jobs.NewJob{RouteID: "r1", Action: dlq.ActionRetry, Match: jobs.Match{IDs: []string{"x"}}, Edit: edit, Pending: true})
	if runner(repo, pages).Tick(ctx) {
		t.Fatal("pending job claimed before approval")
	}
	if _, err := repo.Decide(ctx, pending.ID, jobs.Decision{Approve: true, By: "u2"}); err != nil {
		t.Fatal(err)
	}
	if !runner(repo, pages).Tick(ctx) || len(pages.replayed) != 1 || pages.replayed[0] != "x" {
		t.Fatalf("edited replay %+v", pages.replayed)
	}
	done, _ := repo.Get(ctx, pending.ID)
	if done.Status != jobs.StatusSucceeded || done.Edit != nil || done.Processed != 1 {
		t.Fatalf("done %+v", done)
	}
	raw, _ := json.Marshal(done)
	if strings.Contains(string(raw), `"ids"`) || !strings.Contains(string(raw), `"idsCount":1`) {
		t.Fatalf("job json %s", raw)
	}
}
