package preview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/pkg/kafka"
)

// JanitorPolicy holds the janitor schedule and its cross-replica lock lifetime.
type JanitorPolicy time.Duration

const (
	JanitorInterval JanitorPolicy = JanitorPolicy(15 * time.Minute)
	JanitorLockTTL  JanitorPolicy = JanitorPolicy(10 * time.Minute)
)

// JanitorBatch bounds the topics deleted per tick.
type JanitorBatch int

const JanitorBatchSize JanitorBatch = 100

// JanitorKey is the Redis key of the janitor lock.
type JanitorKey string

const RedisJanitorLock JanitorKey = "preview:janitor"

// JanitorLog names the janitor's log lines.
type JanitorLog string

const (
	LogJanitorDeleted JanitorLog = "preview.test_topic_deleted"
	LogJanitorFailed  JanitorLog = "preview.test_topic_delete_failed"
	LogJanitorTick    JanitorLog = "preview.test_topic_janitor_failed"
)

// JanitorLock lets one replica run a janitor tick.
type JanitorLock interface {
	Acquire(ctx context.Context) (bool, error)
}

// RedisJanitorMutex takes preview:janitor with SET NX and a TTL shorter than the interval.
type RedisJanitorMutex struct {
	Client redis.UniversalClient
}

// Acquire implements JanitorLock.
func (l RedisJanitorMutex) Acquire(ctx context.Context) (bool, error) {
	return l.Client.SetNX(ctx, string(RedisJanitorLock), time.Now().UTC().Format(time.RFC3339), time.Duration(JanitorLockTTL)).Result()
}

// LocalJanitorMutex always runs (single replica, memory session store).
type LocalJanitorMutex struct{}

// Acquire implements JanitorLock.
func (LocalJanitorMutex) Acquire(context.Context) (bool, error) { return true, nil }

// TopicDeleter deletes one topic of a connection; an unknown topic counts as deleted.
type TopicDeleter interface {
	Delete(ctx context.Context, connectionID, topic string) error
}

// AdminTopicDeleter deletes topics with an admin client of the connection.
type AdminTopicDeleter struct {
	Clients ConnectionClients
}

// Delete implements TopicDeleter.
func (d AdminTopicDeleter) Delete(ctx context.Context, connectionID, topic string) error {
	client, err := d.Clients.Open(ctx, connectionID, kafka.PurposeAdmin)
	if err != nil {
		return err
	}
	defer client.Close()
	resp, err := kadm.NewClient(client.Raw()).DeleteTopic(ctx, topic)
	if err == nil {
		err = resp.Err
	}
	if errors.Is(err, kerr.UnknownTopicOrPartition) {
		return nil
	}
	return err
}

// TestTopicJanitor deletes expired test topics every interval; a Redis lock keeps it to one replica.
type TestTopicJanitor struct {
	Registry TestTopicRegistry
	Deleter  TopicDeleter
	Lock     JanitorLock
	Now      func() time.Time
	Log      *slog.Logger
}

// Run ticks until ctx ends (the first tick runs at once).
func (j TestTopicJanitor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(JanitorInterval))
	defer ticker.Stop()
	for {
		if _, err := j.Tick(ctx); err != nil {
			j.Log.Warn(string(LogJanitorTick), string(logging.AttrError), err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick deletes the expired topics once and returns how many were deleted; failures stay for the next tick.
func (j TestTopicJanitor) Tick(ctx context.Context) (int, error) {
	ok, err := j.Lock.Acquire(ctx)
	if err != nil || !ok {
		return 0, err
	}
	now := j.Now().UTC()
	topics, err := j.Registry.Expired(ctx, now, int(JanitorBatchSize))
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, t := range topics {
		if j.remove(ctx, t, now) {
			deleted++
		}
	}
	return deleted, nil
}

func (j TestTopicJanitor) remove(ctx context.Context, t TestTopic, now time.Time) bool {
	if err := j.Deleter.Delete(ctx, t.ConnectionID, t.Topic); err != nil {
		j.Log.Warn(string(LogJanitorFailed), string(logging.AttrError), fmt.Errorf("%s: %w", t.Topic, err))
		return false
	}
	if err := j.Registry.MarkDeleted(ctx, t.ID, now); err != nil {
		j.Log.Warn(string(LogJanitorFailed), string(logging.AttrError), err)
		return false
	}
	j.Log.Info(string(LogJanitorDeleted), string(LogAttrTopic), t.Topic)
	return true
}

// LogAttr names preview log attributes.
type LogAttr string

const LogAttrTopic LogAttr = "topic"
