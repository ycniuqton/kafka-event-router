package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// ReplayClusters opens admin clients on a route's source connection (secrets resolved in the API).
type ReplayClusters interface {
	Admin(ctx context.Context, connectionID string) (kafka.Client, error)
}

// ReplaySource is what a replay resets: the source connection, consumer group and topics of the active version.
type ReplaySource struct {
	ConnectionID string
	Group        string
	Topics       []string
	Pattern      string
}

// Resolve returns the source topics; a topicPattern is matched against the cluster's topics.
func (s ReplaySource) Resolve(ctx context.Context, admin kafka.Admin) ([]string, error) {
	if s.Pattern == "" {
		return s.Topics, nil
	}
	re, err := regexp.Compile(s.Pattern)
	if err != nil {
		return nil, err
	}
	all, err := admin.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, t := range all {
		if !t.Internal && re.MatchString(t.Name) {
			out = append(out, t.Name)
		}
	}
	return out, nil
}

// ReplaySources reads the source of a route's active version.
type ReplaySources struct {
	Routes state.RouteStore
}

// Of parses the active version's config of routeID.
func (s ReplaySources) Of(ctx context.Context, routeID string) (ReplaySource, error) {
	versionID, err := s.Routes.ActiveVersion(ctx, routeID)
	if err != nil {
		return ReplaySource{}, err
	}
	if versionID == "" {
		return ReplaySource{}, ErrNoActiveVersion
	}
	raw, err := s.Routes.VersionConfig(ctx, versionID)
	if err != nil {
		return ReplaySource{}, err
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return ReplaySource{}, err
	}
	cfg, err := doc.Config()
	if err != nil {
		return ReplaySource{}, err
	}
	src := ReplaySource{ConnectionID: cfg.Source.ConnectionID, Group: domain.GroupNaming{}.Of(routeID, cfg.Source.ConsumerGroup), Topics: cfg.Source.Topics}
	if cfg.Source.TopicPattern != nil {
		src.Pattern = *cfg.Source.TopicPattern
	}
	return src, nil
}

// ReplayStep is the base of every replay status handler; one pass advances a replay by at most one status.
type ReplayStep interface {
	Status() domain.ReplayStatus
	Step(ctx context.Context, c *ReplayCoordinator, rp state.Replay) error
}

// ReplayCoordinator moves open replays through stopping → resetting → done (design 5.9), leader only:
// the planner suspends a stopping/resetting route's leases, the group is reset once empty, leases return after.
type ReplayCoordinator struct {
	Stores   state.Stores
	Sources  ReplaySources
	Clusters ReplayClusters
	Timings  domain.Timings
	Log      *slog.Logger
	Now      func() time.Time
	steps    map[domain.ReplayStatus]ReplayStep
}

// NewReplayCoordinator registers the status steps.
func NewReplayCoordinator(stores state.Stores, clusters ReplayClusters, timings domain.Timings, log *slog.Logger, now func() time.Time) *ReplayCoordinator {
	c := &ReplayCoordinator{Stores: stores, Sources: ReplaySources{Routes: stores.Routes}, Clusters: clusters, Timings: timings, Log: log, Now: now,
		steps: map[domain.ReplayStatus]ReplayStep{}}
	for _, s := range []ReplayStep{PendingReplay{}, StoppingReplay{}, ResettingReplay{}} {
		c.steps[s.Status()] = s
	}
	return c
}

// Advance runs one step of every open replay; a failing replay is marked failed and its route restored.
func (c *ReplayCoordinator) Advance(ctx context.Context) error {
	open, err := c.Stores.Replays.Open(ctx)
	if err != nil {
		return err
	}
	for _, rp := range open {
		step, ok := c.steps[rp.Status]
		if !ok {
			continue
		}
		if err := step.Step(ctx, c, rp); err != nil {
			c.finish(ctx, rp, domain.ReplayFailed, err.Error(), 0)
		}
	}
	return nil
}

// move transitions rp to next, recording group and partitions when known.
func (c *ReplayCoordinator) move(ctx context.Context, rp state.Replay, next domain.ReplayStatus, group string, partitions int) error {
	_, err := c.Stores.Replays.Transition(ctx, state.ReplayTransition{ID: rp.ID, From: rp.Status, To: next, At: c.Now(), Group: group, Partitions: partitions})
	return err
}

// finish ends rp as done or failed; the planner gives the route its leases back on its next pass.
func (c *ReplayCoordinator) finish(ctx context.Context, rp state.Replay, end domain.ReplayStatus, message string, partitions int) {
	t := state.ReplayTransition{ID: rp.ID, From: rp.Status, To: end, At: c.Now(), Error: message, Partitions: partitions}
	if _, err := c.Stores.Replays.Transition(ctx, t); err != nil {
		c.Log.Warn("scheduler: replay transition failed", string(logging.AttrRouteID), rp.RouteID, string(logging.AttrError), err)
		return
	}
	c.Log.Info("scheduler: replay finished", string(logging.AttrRouteID), rp.RouteID, string(LogAttrReplayStatus), string(end), string(LogAttrReplayError), message)
}

// expired reports whether a stopping replay waited longer than the replay group wait.
func (c *ReplayCoordinator) expired(rp state.Replay) bool {
	return rp.StartedAt != nil && c.Now().Sub(*rp.StartedAt) > c.Timings.ReplayWait
}

// admin opens an admin client on the route's source connection.
func (c *ReplayCoordinator) admin(ctx context.Context, src ReplaySource) (kafka.Client, error) {
	return c.Clusters.Admin(ctx, src.ConnectionID)
}

// PendingReplay suspends the route: stopping makes the planner remove its leases.
type PendingReplay struct{}

func (PendingReplay) Status() domain.ReplayStatus { return domain.ReplayPending }

// Step implements ReplayStep.
func (PendingReplay) Step(ctx context.Context, c *ReplayCoordinator, rp state.Replay) error {
	return c.move(ctx, rp, domain.ReplayStopping, "", 0)
}

// StoppingReplay waits until no runner holds a lease and the consumer group is empty; members that
// stay longer than the replay wait (foreign consumers of the group) fail the replay.
type StoppingReplay struct{}

func (StoppingReplay) Status() domain.ReplayStatus { return domain.ReplayStopping }

// Step implements ReplayStep.
func (StoppingReplay) Step(ctx context.Context, c *ReplayCoordinator, rp state.Replay) error {
	blocker, group, err := StoppingReplay{}.blocker(ctx, c, rp)
	if err != nil {
		blocker = err.Error()
	}
	if blocker == "" {
		return c.move(ctx, rp, domain.ReplayResetting, group, 0)
	}
	if c.expired(rp) {
		return fmt.Errorf("%s", blocker)
	}
	return nil
}

// blocker names what still prevents the reset ("" when the group is empty and no lease remains).
func (StoppingReplay) blocker(ctx context.Context, c *ReplayCoordinator, rp state.Replay) (string, string, error) {
	leases, err := c.Stores.Assignments.ForRoute(ctx, rp.RouteID)
	if err != nil || len(leases) > 0 {
		return fmt.Sprintf(string(ReplayMsgRunnersAssigned), len(leases)), "", err
	}
	src, err := c.Sources.Of(ctx, rp.RouteID)
	if err != nil {
		return "", "", err
	}
	admin, err := c.admin(ctx, src)
	if err != nil {
		return "", "", err
	}
	defer admin.Close()
	info, err := admin.Admin().DescribeGroup(ctx, src.Group)
	if err != nil {
		return "", "", err
	}
	if !info.Empty() {
		return fmt.Sprintf(string(ReplayMsgGroupMembers), src.Group, len(info.Members)), src.Group, nil
	}
	return "", src.Group, nil
}

// ResettingReplay commits, for every source partition, the first offset at or after the replay's from.
type ResettingReplay struct{}

func (ResettingReplay) Status() domain.ReplayStatus { return domain.ReplayResetting }

// Step implements ReplayStep.
func (ResettingReplay) Step(ctx context.Context, c *ReplayCoordinator, rp state.Replay) error {
	src, err := c.Sources.Of(ctx, rp.RouteID)
	if err != nil {
		return err
	}
	admin, err := c.admin(ctx, src)
	if err != nil {
		return err
	}
	defer admin.Close()
	topics, err := src.Resolve(ctx, admin.Admin())
	if err != nil {
		return err
	}
	offsets, err := admin.Admin().OffsetsForTimes(ctx, rp.From, topics...)
	if err != nil {
		return err
	}
	if err := admin.Admin().CommitOffsets(ctx, src.Group, offsets); err != nil {
		return err
	}
	c.finish(ctx, rp, domain.ReplayDone, "", PartitionCount(offsets))
	return nil
}

// PartitionCount counts the partitions of offsets.
func PartitionCount(offsets kafka.PartitionOffsets) int {
	n := 0
	for _, ps := range offsets {
		n += len(ps)
	}
	return n
}

// ReplayMessage holds the replay failure texts shown to users.
type ReplayMessage string

const (
	ReplayMsgRunnersAssigned ReplayMessage = "%d runners still assigned to the route"
	ReplayMsgGroupMembers    ReplayMessage = "consumer group %s still has %d members"
)

// LogAttrReplay names replay log attributes.
const (
	LogAttrReplayStatus LogAttr = "replay_status"
	LogAttrReplayError  LogAttr = "replay_error"
)
