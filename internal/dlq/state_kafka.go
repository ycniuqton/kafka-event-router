package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/kafka"
)

// CompactedTopicStore materializes router.dlq.state per DLQ connection: bootstrap from the
// earliest offset, a tail goroutine for other instances' writes, read-your-writes on Mark.
type CompactedTopicStore struct {
	clients ClientOpener
	log     *slog.Logger
	root    context.Context
	mu      sync.Mutex
	conns   map[string]*StateConn
}

// NewCompactedTopicStore builds the store; tails stop when root is cancelled.
func NewCompactedTopicStore(root context.Context, clients ClientOpener, log *slog.Logger) *CompactedTopicStore {
	return &CompactedTopicStore{clients: clients, log: log, root: root, conns: map[string]*StateConn{}}
}

// StateConn is the index of one connection plus its consumer/producer client.
type StateConn struct {
	index  *StateIndex
	client *kgo.Client
	ready  chan struct{}
	err    error
}

// conn returns the ready state of a connection, starting it on first use (design §5.10).
func (s *CompactedTopicStore) conn(ctx context.Context, id string) (*StateConn, error) {
	s.mu.Lock()
	c, ok := s.conns[id]
	if !ok {
		c = &StateConn{index: NewStateIndex(), ready: make(chan struct{})}
		s.conns[id] = c
		go s.start(id, c)
	}
	s.mu.Unlock()
	wait, cancel := context.WithTimeout(ctx, IndexReadyWait.Duration())
	defer cancel()
	select {
	case <-c.ready:
	case <-wait.Done():
		return nil, ErrStateUnavailable
	}
	if c.err != nil {
		s.forget(id, c)
		return nil, c.err
	}
	return c, nil
}

func (s *CompactedTopicStore) forget(id string, c *StateConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns[id] == c {
		delete(s.conns, id)
	}
}

// start ensures the topic, opens the client and runs bootstrap + tail.
func (s *CompactedTopicStore) start(id string, c *StateConn) {
	if err := (StateTopicEnsurer{Clients: s.clients}).Ensure(s.root, id); err != nil {
		c.err = err
		close(c.ready)
		return
	}
	cl, err := s.clients.Open(s.root, id, kafka.PurposeConsumer,
		kgo.ConsumeTopics(string(StateTopicName)), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		c.err = err
		close(c.ready)
		return
	}
	c.client = cl
	StateTail{Conn: c, Log: s.log}.Run(s.root)
}

// Lookup implements StateStore.
func (s *CompactedTopicStore) Lookup(ctx context.Context, conn, routeID string, refs []Ref) (map[Ref]Resolution, error) {
	c, err := s.conn(ctx, conn)
	if err != nil {
		return nil, err
	}
	return c.index.Lookup(routeID, refs), nil
}

// CountResolved implements StateStore.
func (s *CompactedTopicStore) CountResolved(ctx context.Context, conn, routeID string, ranges []PartitionRange) (int, error) {
	c, err := s.conn(ctx, conn)
	if err != nil {
		return 0, err
	}
	return c.index.Count(routeID, ranges), nil
}

// Expired implements StateStore.
func (s *CompactedTopicStore) Expired(ctx context.Context, conn, routeID string, ranges []PartitionRange) ([]Ref, error) {
	c, err := s.conn(ctx, conn)
	if err != nil {
		return nil, err
	}
	return c.index.Below(routeID, ranges), nil
}

// Mark implements StateStore: produce (acks=all, idempotent), then apply locally.
func (s *CompactedTopicStore) Mark(ctx context.Context, conn string, rs []Resolution) error {
	c, err := s.conn(ctx, conn)
	if err != nil {
		return err
	}
	recs := make([]*kgo.Record, 0, len(rs))
	for _, r := range rs {
		value, _ := json.Marshal(r)
		recs = append(recs, &kgo.Record{Topic: string(StateTopicName), Key: []byte((StateKey{}).Of(r.RouteID, r.Ref)), Value: value})
	}
	if err := c.client.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		return ErrUpstream.Wrap(err)
	}
	for _, r := range rs {
		c.index.Put(r)
	}
	return nil
}

// Clear implements StateStore with tombstones.
func (s *CompactedTopicStore) Clear(ctx context.Context, conn, routeID string, refs []Ref) error {
	c, err := s.conn(ctx, conn)
	if err != nil {
		return err
	}
	recs := make([]*kgo.Record, 0, len(refs))
	for _, ref := range refs {
		recs = append(recs, &kgo.Record{Topic: string(StateTopicName), Key: []byte((StateKey{}).Of(routeID, ref))})
	}
	if err := c.client.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		return ErrUpstream.Wrap(err)
	}
	for _, ref := range refs {
		c.index.Delete(routeID, ref)
	}
	return nil
}

// StateTopicEnsurer creates router.dlq.state (compact, 6 partitions) when missing.
type StateTopicEnsurer struct {
	Clients ClientOpener
}

// Ensure creates the topic; an existing topic is fine, any other failure is upstream.
func (e StateTopicEnsurer) Ensure(ctx context.Context, conn string) error {
	cl, err := e.Clients.Open(ctx, conn, kafka.PurposeAdmin)
	if err != nil {
		return err
	}
	defer cl.Close()
	compact := string(CleanupCompact)
	resp, err := kadm.NewClient(cl).CreateTopic(ctx, int32(StateTopicPartitions), int16(StateTopicReplication),
		map[string]*string{string(ConfigCleanupPolicy): &compact}, string(StateTopicName))
	if err == nil {
		err = resp.Err
	}
	if err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
		return ErrUpstream.Wrap(err)
	}
	return nil
}

// StateTail applies state records: bootstrap until caught up (ready), then follow forever.
type StateTail struct {
	Conn *StateConn
	Log  *slog.Logger
}

// Run blocks until ctx is cancelled.
func (t StateTail) Run(ctx context.Context) {
	defer t.Conn.client.Close()
	t.bootstrap(ctx)
	close(t.Conn.ready)
	for ctx.Err() == nil {
		fetches := t.Conn.client.PollFetches(ctx)
		if t.failed(fetches) {
			t.sleep(ctx)
			continue
		}
		fetches.EachRecord(func(r *kgo.Record) { t.Conn.index.Apply(r.Key, r.Value) })
	}
}

// bootstrap applies records until a poll stays idle (the index caught up).
func (t StateTail) bootstrap(ctx context.Context) {
	for ctx.Err() == nil {
		poll, cancel := context.WithTimeout(ctx, IdlePoll.Duration())
		fetches := t.Conn.client.PollFetches(poll)
		cancel()
		if fetches.NumRecords() == 0 {
			return
		}
		fetches.EachRecord(func(r *kgo.Record) { t.Conn.index.Apply(r.Key, r.Value) })
	}
}

func (t StateTail) failed(fetches kgo.Fetches) bool {
	for _, fe := range fetches.Errors() {
		if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, kgo.ErrClientClosed) {
			return false
		}
		t.Log.Warn(string(LogStateTail), string(LogAttrError), fe.Err)
		return true
	}
	return false
}

func (t StateTail) sleep(ctx context.Context) {
	timer := time.NewTimer(TailReconnect.Duration())
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// LogEvent names the feature's log events (payloads are never logged).
type LogEvent string

const (
	LogStateTail      LogEvent = "dlq.state_tail_error"
	LogStateNotSaved  LogEvent = "dlq.state_not_saved"
	LogScanFailed     LogEvent = "dlq.scan_failed"
	LogDisabled       LogEvent = "dlq module disabled"
	LogDemoSeedFailed LogEvent = "dlq.demo_seed_failed"
)

// LogAttr names log attributes.
type LogAttr string

const (
	LogAttrError LogAttr = "error"
	LogAttrRoute LogAttr = "route_id"
)
