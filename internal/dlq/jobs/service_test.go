package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
)

// fixedPreparer accepts every job for one route.
type fixedPreparer struct{ route dlq.RouteInfo }

func (p fixedPreparer) PrepareJob(context.Context, dlq.ActionKind, string) (dlq.RouteInfo, error) {
	return p.route, nil
}

// recordingTrail keeps every audited event.
type recordingTrail struct{ events []dlq.AuditEvent }

func (r *recordingTrail) Record(_ context.Context, e dlq.AuditEvent) error {
	r.events = append(r.events, e)
	return nil
}

// TestServiceAuditsCreateAndCancel checks that job requests are audited by the service itself.
func TestServiceAuditsCreateAndCancel(t *testing.T) {
	ctx := context.Background()
	route := dlq.RouteInfo{ID: "r1", Name: "orders", TeamID: "t1"}
	trail := &recordingTrail{}
	svc := jobs.Service{Repo: jobs.NewMemoryRepository(time.Now), Preparer: fixedPreparer{route: route}, Audit: trail}
	job, err := svc.Create(ctx, dlq.ActionDiscard, dlq.Actor{UserID: "u1"}, route.ID, dlq.Query{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Cancel(ctx, job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	want := []dlq.AuditEvent{{Action: dlq.ActionDLQJobCreated}, {Action: dlq.ActionDLQJobCancelled}}
	if len(trail.events) != len(want) {
		t.Fatalf("audited %d events, want %d", len(trail.events), len(want))
	}
	for i, w := range want {
		got := trail.events[i]
		if got.Action != w.Action || got.Route.ID != route.ID {
			t.Errorf("event %d = %s on %s, want %s on %s", i, got.Action, got.Route.ID, w.Action, route.ID)
		}
	}
}

// TestServiceWithoutAuditTrail keeps working when no trail is configured.
func TestServiceWithoutAuditTrail(t *testing.T) {
	svc := jobs.Service{Repo: jobs.NewMemoryRepository(time.Now), Preparer: fixedPreparer{route: dlq.RouteInfo{ID: "r1"}}}
	if _, err := svc.Create(context.Background(), dlq.ActionRetry, dlq.Actor{}, "r1", dlq.Query{}); err != nil {
		t.Fatalf("create: %v", err)
	}
}
