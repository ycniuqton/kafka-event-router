package dlq

import (
	"context"
	"log/slog"
	"time"
)

// JanitorTiming holds the state janitor's schedule.
type JanitorTiming time.Duration

const (
	JanitorInterval   JanitorTiming = JanitorTiming(24 * time.Hour)
	JanitorFirstDelay JanitorTiming = JanitorTiming(5 * time.Minute)
)

// Duration returns the timing as a time.Duration.
func (t JanitorTiming) Duration() time.Duration { return time.Duration(t) }

// Leadership elects one API instance (scheduler.PgLeaderElector with its own advisory lock key).
type Leadership interface {
	Campaign(ctx context.Context) (lost <-chan struct{}, err error)
}

// JanitorLockKey is the advisory lock id of the DLQ state janitor (7001 migrations, 7008 scheduler).
type JanitorLockKey int64

const StateJanitorLockKey JanitorLockKey = 7110

// JanitorLog names janitor log messages and attributes.
type JanitorLog string

const (
	LogJanitorSwept JanitorLog = "dlq janitor: tombstoned expired state keys"
	LogJanitorError JanitorLog = "dlq janitor: route skipped"
	LogAttrRemoved  JanitorLog = "removed"
	LogAttrRouteID  JanitorLog = "route_id"
	LogAttrErr      JanitorLog = "error"
)

// StateJanitor keeps router.dlq.state small (DL-115): daily, on the leader only, resolutions whose
// DLQ offset fell below the topic's low watermark get tombstones.
type StateJanitor struct {
	Routes  RouteSource
	Locator Locator
	Reader  Reader
	State   StateStore
	Leader  Leadership
	Log     *slog.Logger
}

// Run sweeps after JanitorFirstDelay and then every JanitorInterval until ctx ends.
func (j StateJanitor) Run(ctx context.Context) {
	timer := time.NewTimer(JanitorFirstDelay.Duration())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			j.Once(ctx)
			timer.Reset(JanitorInterval.Duration())
		}
	}
}

// Once sweeps every route when this instance wins leadership; it returns the tombstones written.
func (j StateJanitor) Once(ctx context.Context) int {
	lead, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := j.Leader.Campaign(lead); err != nil {
		return 0
	}
	routes, err := j.Routes.Routes(lead)
	if err != nil {
		j.Log.Warn(string(LogJanitorError), string(LogAttrErr), err)
		return 0
	}
	removed := 0
	for _, r := range routes {
		removed += j.route(lead, r.ID)
	}
	j.Log.Info(string(LogJanitorSwept), string(LogAttrRemoved), removed)
	return removed
}

// route tombstones the expired resolutions of one route on each of its DLQ connections.
func (j StateJanitor) route(ctx context.Context, routeID string) int {
	loc, err := j.Locator.Locate(ctx, routeID)
	if err != nil {
		return 0
	}
	removed := 0
	for _, t := range loc.Targets {
		n, err := j.target(ctx, routeID, t)
		if err != nil {
			j.Log.Warn(string(LogJanitorError), string(LogAttrRouteID), routeID, string(LogAttrErr), err)
		}
		removed += n
	}
	return removed
}

func (j StateJanitor) target(ctx context.Context, routeID string, t Target) (int, error) {
	set, err := j.Reader.Ranges(ctx, []Target{t})
	if err != nil {
		return 0, err
	}
	expired, err := j.State.Expired(ctx, t.ConnectionID, routeID, set.Ranges)
	if err != nil || len(expired) == 0 {
		return 0, err
	}
	return len(expired), j.State.Clear(ctx, t.ConnectionID, routeID, expired)
}

// AlwaysLeader is the single-instance leadership (tests, deployments without Postgres).
type AlwaysLeader struct{}

// Campaign implements Leadership.
func (AlwaysLeader) Campaign(ctx context.Context) (<-chan struct{}, error) { return ctx.Done(), nil }
