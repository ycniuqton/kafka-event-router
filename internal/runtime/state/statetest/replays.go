package statetest

import (
	"context"
	"fmt"
	"sort"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

type replays struct{ m *Memory }

func (r replays) Create(_ context.Context, rp state.Replay) (state.Replay, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for _, cur := range r.m.Replays {
		if cur.RouteID == rp.RouteID && cur.Status.Open() {
			return state.Replay{}, store.ErrConflict
		}
	}
	rp.ID = fmt.Sprintf("replay-%d", len(r.m.Replays)+1)
	rp.Status = domain.ReplayPending
	r.m.Replays[rp.ID] = rp
	return rp, nil
}

func (r replays) Get(_ context.Context, routeID, id string) (state.Replay, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	rp, ok := r.m.Replays[id]
	if !ok || rp.RouteID != routeID {
		return state.Replay{}, store.ErrNotFound
	}
	return rp, nil
}

func (r replays) Open(context.Context) ([]state.Replay, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := []state.Replay{}
	for _, rp := range r.m.Replays {
		if rp.Status.Open() {
			out = append(out, rp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r replays) Transition(_ context.Context, t state.ReplayTransition) (bool, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	rp, ok := r.m.Replays[t.ID]
	if !ok || rp.Status != t.From {
		return false, nil
	}
	at := t.At
	rp.Status, rp.Error = t.To, t.Error
	if t.To == domain.ReplayStopping {
		rp.StartedAt = &at
	}
	if !t.To.Open() {
		rp.FinishedAt = &at
	}
	if t.Group != "" {
		rp.ConsumerGroup = t.Group
	}
	rp.Partitions = max(rp.Partitions, t.Partitions)
	r.m.Replays[t.ID] = rp
	return true, nil
}
