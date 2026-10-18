package jobs

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/platform/problem"
)

// ErrNotFound is returned for unknown job ids (404).
var ErrNotFound = problem.NewError(problem.KindNotFound, "This dead-letter job does not exist.")

// NewJob is what a job is created from.
type NewJob struct {
	RouteID     string
	Action      dlq.ActionKind
	Match       Match
	RequestedBy string
}

// Repository persists jobs (DL-110): create, claim, progress, heartbeat, finish, cancel, reap.
type Repository interface {
	Create(ctx context.Context, n NewJob) (Job, error)
	Get(ctx context.Context, id string) (Job, error)
	ListByRoute(ctx context.Context, routeID string, limit int) ([]Job, error)
	Claim(ctx context.Context) (Job, bool, error)
	Progress(ctx context.Context, j Job) (Status, error)
	Heartbeat(ctx context.Context, id string) error
	Finish(ctx context.Context, id string, st Status, errText string) (Job, error)
	RequestCancel(ctx context.Context, id string) (Job, error)
	Requeue(ctx context.Context, staleBefore time.Time) (int, error)
}

// MemoryRepository keeps jobs in process memory (tests, deployments without Postgres).
type MemoryRepository struct {
	mu   sync.Mutex
	jobs map[string]*Job
	now  func() time.Time
}

// NewMemoryRepository returns an empty repository.
func NewMemoryRepository(now func() time.Time) *MemoryRepository {
	return &MemoryRepository{jobs: map[string]*Job{}, now: now}
}

// Create implements Repository.
func (r *MemoryRepository) Create(_ context.Context, n NewJob) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := &Job{ID: uuid.NewString(), RouteID: n.RouteID, Action: n.Action, Match: n.Match, Status: StatusQueued,
		Counts: map[dlq.Outcome]int{}, RequestedBy: n.RequestedBy, CreatedAt: r.now()}
	r.jobs[j.ID] = j
	return *j, nil
}

// Get implements Repository.
func (r *MemoryRepository) Get(_ context.Context, id string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return *j, nil
}

// ListByRoute implements Repository (newest first).
func (r *MemoryRepository) ListByRoute(_ context.Context, routeID string, limit int) ([]Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Job{}
	for _, j := range r.jobs {
		if j.RouteID == routeID {
			out = append(out, *j)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	return out[:min(len(out), limit)], nil
}

// Claim implements Repository: the oldest queued job becomes running.
func (r *MemoryRepository) Claim(_ context.Context) (Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var next *Job
	for _, j := range r.jobs {
		if j.Status == StatusQueued && (next == nil || j.CreatedAt.Before(next.CreatedAt)) {
			next = j
		}
	}
	if next == nil {
		return Job{}, false, nil
	}
	now := r.now()
	next.Status, next.HeartbeatAt = StatusRunning, &now
	if next.StartedAt == nil {
		next.StartedAt = &now
	}
	return *next, true, nil
}

// Progress implements Repository and returns the stored status (cancel requests).
func (r *MemoryRepository) Progress(_ context.Context, j Job) (Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.jobs[j.ID]
	if !ok {
		return "", ErrNotFound
	}
	now := r.now()
	cur.Processed, cur.Counts, cur.Cursor, cur.Total, cur.HeartbeatAt = j.Processed, j.Counts, j.Cursor, max(cur.Total, j.Total), &now
	return cur.Status, nil
}

// Heartbeat implements Repository.
func (r *MemoryRepository) Heartbeat(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		now := r.now()
		j.HeartbeatAt = &now
	}
	return nil
}

// Finish implements Repository.
func (r *MemoryRepository) Finish(_ context.Context, id string, st Status, errText string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	now := r.now()
	j.Status, j.Error, j.FinishedAt = st, errText, &now
	return *j, nil
}

// RequestCancel implements Repository: queued jobs are cancelled at once, running ones asked to stop.
func (r *MemoryRepository) RequestCancel(_ context.Context, id string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	switch j.Status {
	case StatusQueued:
		now := r.now()
		j.Status, j.FinishedAt = StatusCancelled, &now
	case StatusRunning:
		j.Status = StatusCancelRequested
	}
	return *j, nil
}

// Requeue implements Repository: running jobs without a recent heartbeat go back to the queue.
func (r *MemoryRepository) Requeue(_ context.Context, staleBefore time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, j := range r.jobs {
		if j.Status == StatusRunning && j.HeartbeatAt != nil && j.HeartbeatAt.Before(staleBefore) {
			j.Status = StatusQueued
			n++
		}
	}
	return n, nil
}

// RepositoryFactory builds a repository.
type RepositoryFactory func() Repository

// RepositoryRegistry selects the configured job store (ROUTER_DLQ_JOB_STORE).
type RepositoryRegistry struct {
	factories map[dlq.JobStore]RepositoryFactory
}

// NewRepositoryRegistry returns an empty registry.
func NewRepositoryRegistry() *RepositoryRegistry {
	return &RepositoryRegistry{factories: map[dlq.JobStore]RepositoryFactory{}}
}

// Register adds a store.
func (r *RepositoryRegistry) Register(s dlq.JobStore, f RepositoryFactory) *RepositoryRegistry {
	r.factories[s] = f
	return r
}

// Build returns the repository of store s.
func (r *RepositoryRegistry) Build(s dlq.JobStore) (Repository, error) {
	f, ok := r.factories[s]
	if !ok {
		return nil, fmt.Errorf("dlq jobs: no job store %q", s)
	}
	return f(), nil
}
