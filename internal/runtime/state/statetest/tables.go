package statetest

import (
	"context"
	"sort"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

type assignments struct{ m *Memory }

func (a assignments) ForWorker(_ context.Context, workerID string) ([]domain.Assignment, error) {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	out := []domain.Assignment{}
	for _, l := range a.m.Leases {
		if l.WorkerID == workerID && l.TargetVersionID != "" {
			out = append(out, domain.Assignment{RouteID: l.RouteID, RouteName: a.m.RouteNames[l.RouteID], VersionID: l.TargetVersionID,
				VersionNo: a.m.VersionNos[l.TargetVersionID], LeaseExpiresAt: l.LeaseExpiresAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RouteID < out[j].RouteID })
	return out, nil
}

func (a assignments) ReportStatus(_ context.Context, workerID string, replicas []domain.ReplicaStatus, now time.Time) error {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	for _, s := range replicas {
		k := [2]string{s.RouteID, workerID}
		l, ok := a.m.Leases[k]
		if !ok {
			continue
		}
		l.ReportedVersionID, l.RunnerState, l.ErrorCategory, l.ErrorMessage = s.VersionID, s.State, s.ErrorCategory, s.ErrorMessage
		l.Lag, l.Partitions, l.ReportedAt = s.Lag, s.Partitions, &now
		a.m.Leases[k] = l
	}
	return nil
}

func (a assignments) List(context.Context) ([]state.AssignmentRow, error) {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	out := []state.AssignmentRow{}
	for _, l := range a.m.Leases {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RouteID+out[i].WorkerID < out[j].RouteID+out[j].WorkerID })
	return out, nil
}

func (a assignments) ForRoute(ctx context.Context, routeID string) ([]state.AssignmentRow, error) {
	all, _ := a.List(ctx)
	out := []state.AssignmentRow{}
	for _, l := range all {
		if l.RouteID == routeID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (a assignments) Insert(_ context.Context, row state.AssignmentRow) error {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	k := [2]string{row.RouteID, row.WorkerID}
	if _, ok := a.m.Leases[k]; !ok {
		a.m.Leases[k] = row
	}
	return nil
}

func (a assignments) Delete(_ context.Context, routeID, workerID string) error {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	delete(a.m.Leases, [2]string{routeID, workerID})
	return nil
}

func (a assignments) DeleteForWorker(ctx context.Context, workerID string, routeIDs []string) error {
	for _, id := range routeIDs {
		_ = a.Delete(ctx, id, workerID)
	}
	return nil
}

func (a assignments) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	n := 0
	for k, l := range a.m.Leases {
		if l.LeaseExpiresAt.Before(now) {
			delete(a.m.Leases, k)
			n++
		}
	}
	return n, nil
}

func (a assignments) SetTarget(_ context.Context, routeID, workerID, versionID string) error {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	k := [2]string{routeID, workerID}
	if l, ok := a.m.Leases[k]; ok {
		l.TargetVersionID = versionID
		a.m.Leases[k] = l
	}
	return nil
}

type routes struct{ m *Memory }

func (r routes) Demands(context.Context) ([]state.RouteDemand, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := []state.RouteDemand{}
	for _, d := range r.m.Demands {
		if rt, ok := r.m.Runtime[d.RouteID]; ok {
			d.HaltedAt = rt.HaltedAt
		}
		for _, rp := range r.m.Replays {
			d.Replaying = d.Replaying || (rp.RouteID == d.RouteID && rp.Status.Suspends())
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RouteID < out[j].RouteID })
	return out, nil
}

func (r routes) VersionConfig(_ context.Context, id string) ([]byte, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	doc, ok := r.m.Configs[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return doc, nil
}

func (r routes) VersionNo(_ context.Context, id string) (int, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	no, ok := r.m.VersionNos[id]
	if !ok {
		return 0, store.ErrNotFound
	}
	return no, nil
}

func (r routes) Exists(_ context.Context, id string) (bool, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	_, ok := r.m.Demands[id]
	return ok, nil
}

func (r routes) ActiveVersion(_ context.Context, id string) (string, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	d, ok := r.m.Demands[id]
	if !ok {
		return "", store.ErrNotFound
	}
	return d.ActiveVersionID, nil
}

type runtime struct{ m *Memory }

func (r runtime) Get(_ context.Context, id string) (state.RuntimeRow, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	row, ok := r.m.Runtime[id]
	if !ok {
		return state.RuntimeRow{}, store.ErrNotFound
	}
	return row, nil
}

func (r runtime) Save(_ context.Context, row state.RuntimeRow) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	cur := r.m.Runtime[row.RouteID]
	row.HaltedAt, row.HaltReason = cur.HaltedAt, cur.HaltReason
	r.m.Runtime[row.RouteID] = row
	return nil
}

func (r runtime) Halt(_ context.Context, h domain.HaltReport, now time.Time) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	row := r.m.Runtime[h.RouteID]
	row.RouteID, row.Observed, row.ErrorMessage, row.ErrorCategory, row.HaltReason = h.RouteID, domain.ObservedError, h.Message, h.ErrorCategory, h.Reason
	if row.HaltedAt == nil {
		row.HaltedAt = &now
	}
	r.m.Runtime[h.RouteID] = row
	return nil
}

func (r runtime) ClearHalt(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if row, ok := r.m.Runtime[id]; ok {
		row.HaltedAt, row.HaltReason, row.ErrorMessage, row.ErrorCategory = nil, "", "", ""
		r.m.Runtime[id] = row
	}
	return nil
}

type rollouts struct{ m *Memory }

func (r rollouts) Rolling(context.Context) ([]state.Rollout, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := []state.Rollout{}
	for _, d := range r.m.Deployments {
		if d.Status == string(StatusRolling) && r.m.Demands[d.RouteID].ActiveVersionID == d.VersionID {
			out = append(out, d.Rollout)
		}
	}
	return out, nil
}

func (r rollouts) MarkActivated(_ context.Context, id string, at time.Time) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if d, ok := r.m.Deployments[id]; ok && d.ActivatedAt == nil {
		d.ActivatedAt = &at
	}
	return nil
}

func (r rollouts) SupersedeActive(_ context.Context, routeID, except string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for id, d := range r.m.Deployments {
		if d.RouteID == routeID && id != except && d.Status == string(StatusActive) {
			d.Status = string(StatusSuperseded)
		}
	}
	return nil
}
