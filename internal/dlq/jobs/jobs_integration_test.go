package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/dlqtest"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/testkit"
)

// crashingRepo loses the progress write of the second page: the page was applied but its cursor never stored,
// which is what a runner crash between apply and persist leaves behind.
type crashingRepo struct {
	jobs.Repository
	writes atomic.Int32
}

func (r *crashingRepo) Progress(ctx context.Context, j jobs.Job) (jobs.Status, error) {
	if r.writes.Add(1) == 2 {
		return "", errors.New("runner crashed")
	}
	return r.Repository.Progress(ctx, j)
}

func seedRoute(t *testing.T, sb *testkit.PostgresSandbox) {
	t.Helper()
	ctx := context.Background()
	var team string
	if err := sb.DB.Pool().QueryRow(ctx, `INSERT INTO teams (name) VALUES ('dlq-it') RETURNING id::text`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	if _, err := sb.DB.Pool().Exec(ctx, `INSERT INTO routes (id, team_id, name) VALUES ($1, $2, 'orders')`, dlqtest.RouteID, team); err != nil {
		t.Fatal(err)
	}
}

// TestJobResumesFromCursorAfterCrash is DL-124: Postgres queue + Kafka DLQ; a crash after page 2 was applied
// requeues the job (reaper) and another runner resumes from page 1's cursor without acting on any record twice.
func TestJobResumesFromCursorAfterCrash(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	seedRoute(t, sb)
	opener := dlqtest.Cluster(t)
	values := make([]string, 2*int(dlq.MaxSyncAction)+200)
	for i := range values {
		values[i] = fmt.Sprintf(`{"n":%d}`, i)
	}
	dlqtest.Produce(t, opener, values...)
	m := dlqtest.Manager(t, opener)
	repo := jobs.NewPostgresRepository(sb.DB)
	ctx, log := context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil))
	job, err := (jobs.Service{Repo: repo, Preparer: m}).Create(ctx, dlq.ActionDiscard, dlq.Actor{}, dlqtest.RouteID, dlq.Query{})
	if err != nil {
		t.Fatal(err)
	}
	crashed := &jobs.Runner{Repo: &crashingRepo{Repository: repo}, Pages: m, Notifier: jobs.Notifiers{}, Log: log, Now: time.Now}
	if !crashed.Tick(ctx) {
		t.Fatal("job not claimed")
	}
	if stuck, _ := repo.Get(ctx, job.ID); stuck.Status != jobs.StatusRunning || stuck.Processed != int(dlq.MaxSyncAction) {
		t.Fatalf("after crash %+v", stuck)
	}
	resumed := &jobs.Runner{Repo: repo, Pages: m, Notifier: jobs.Notifiers{}, Log: log, Now: func() time.Time { return time.Now().Add(time.Hour) }}
	resumed.Reap(ctx)
	if !resumed.Tick(ctx) {
		t.Fatal("requeued job not claimed")
	}
	done, _ := repo.Get(ctx, job.ID)
	if done.Status != jobs.StatusSucceeded || done.Counts[dlq.OutcomeSkipped] != 0 || done.Counts[dlq.OutcomeFailed] != 0 || done.Total != len(values) {
		t.Fatalf("resumed job %+v", done)
	}
	if page, _ := m.List(ctx, dlqtest.RouteID, dlq.Query{}); len(page.Items) != 0 {
		t.Fatalf("%d dead letters left", len(page.Items))
	}
}
