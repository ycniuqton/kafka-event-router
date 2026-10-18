package runtimestore

import (
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// New returns every route-runtime port over db.
func New(db *store.DB, tx store.TxManager) state.Stores {
	return state.Stores{Workers: NewWorkerRepository(db), Assignments: NewAssignmentRepository(db), Routes: NewRouteRepository(db),
		Runtime: NewRuntimeRepository(db), Rollouts: NewRolloutRepository(db), Replays: NewReplayRepository(db), Tx: tx}
}
