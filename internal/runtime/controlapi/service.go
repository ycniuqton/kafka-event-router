// Package controlapi serves route-runtime's HTTP surface inside cmd/api: the internal worker API under
// /internal/v1 (heartbeat, assignments, version configs, status, halts, change stream) and the public
// GET /api/v1/routes/{id}/runtime and GET /api/v1/workers; RuntimeStatusAggregator folds replica statuses.
package controlapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// WorkerAPI is the business side of the internal worker endpoints.
type WorkerAPI struct {
	Stores     state.Stores
	Aggregator *RuntimeStatusAggregator
	Timings    domain.Timings
	Now        func() time.Time
}

// Heartbeat upserts the worker, extends its leases and drops refused routes (replanned elsewhere).
func (s *WorkerAPI) Heartbeat(ctx context.Context, workerID string, hb domain.Heartbeat) (domain.HeartbeatReply, error) {
	now := s.Now()
	err := s.Stores.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Stores.Workers.UpsertHeartbeat(ctx, workerID, hb, now); err != nil {
			return err
		}
		if err := s.Stores.Workers.ExtendLeases(ctx, workerID, now.Add(s.Timings.LeaseTTL)); err != nil {
			return err
		}
		if len(hb.RefusedRouteIDs) == 0 {
			return nil
		}
		return s.Stores.Assignments.DeleteForWorker(ctx, workerID, hb.RefusedRouteIDs)
	})
	if err != nil {
		return domain.HeartbeatReply{}, err
	}
	set, err := s.Assignments(ctx, workerID)
	if err != nil {
		return domain.HeartbeatReply{}, err
	}
	return domain.HeartbeatReply{LeaseTTLMs: s.Timings.LeaseTTL.Milliseconds(), HeartbeatMs: s.Timings.Heartbeat.Milliseconds(), AssignmentsETag: set.ETag}, nil
}

// Assignments returns the worker's explicit assignment set with a content ETag (leases excluded).
func (s *WorkerAPI) Assignments(ctx context.Context, workerID string) (domain.AssignmentSet, error) {
	list, err := s.Stores.Assignments.ForWorker(ctx, workerID)
	if err != nil {
		return domain.AssignmentSet{}, err
	}
	return domain.AssignmentSet{ETag: AssignmentETag{}.Of(list), Assignments: list}, nil
}

// VersionConfig returns an immutable version document.
func (s *WorkerAPI) VersionConfig(ctx context.Context, versionID string) ([]byte, error) {
	return s.Stores.Routes.VersionConfig(ctx, versionID)
}

// ReportStatus stores replica statuses and refreshes route_runtime of every reported route.
func (s *WorkerAPI) ReportStatus(ctx context.Context, workerID string, replicas []domain.ReplicaStatus) error {
	if err := s.Stores.Assignments.ReportStatus(ctx, workerID, replicas, s.Now()); err != nil {
		return err
	}
	for _, r := range replicas {
		if err := s.Aggregator.Refresh(ctx, r.RouteID); err != nil {
			return err
		}
	}
	return nil
}

// AssignmentETag fingerprints the (route, version) pairs of a set.
type AssignmentETag struct{}

// Of returns a quoted strong ETag.
func (AssignmentETag) Of(list []domain.Assignment) string {
	keys := make([]string, 0, len(list))
	for _, a := range list {
		keys = append(keys, a.RouteID+"@"+a.VersionID)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
	}
	return strconv.Quote(hex.EncodeToString(h.Sum(nil))[:int(ETagLength)])
}

// ETagSize is the hex length of assignment ETags.
type ETagSize int

const ETagLength ETagSize = 16
