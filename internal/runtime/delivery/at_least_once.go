package delivery

import (
	"context"
	"errors"
	"sync"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// AtLeastOnceFactory builds AtLeastOnceStrategy.
type AtLeastOnceFactory struct{}

func (AtLeastOnceFactory) Mode() config.DeliveryMode { return config.DeliveryModeAtLeastOnce }

// New leases one idempotent acks=all producer per output connection.
func (AtLeastOnceFactory) New(ctx context.Context, d Deps) (Strategy, error) {
	return &AtLeastOnceStrategy{deps: d, leases: map[string]*producers.Lease{}}, nil
}

// AtLeastOnceStrategy produces with idempotent acks=all producers and commits offsets synchronously after acks.
type AtLeastOnceStrategy struct {
	deps   Deps
	mu     sync.Mutex
	leases map[string]*producers.Lease
}

func (*AtLeastOnceStrategy) Mode() config.DeliveryMode { return config.DeliveryModeAtLeastOnce }

// Batching implements Strategy: records flow across polls, offsets follow acks.
func (*AtLeastOnceStrategy) Batching() BatchMode { return BatchPipelined }

// BeginBatch implements Strategy (no transaction).
func (*AtLeastOnceStrategy) BeginBatch(context.Context) error { return nil }

// EndBatch implements Strategy (no transaction; nothing to commit here).
func (*AtLeastOnceStrategy) EndBatch(context.Context, bool) (bool, error) { return true, nil }

// Produce sends out asynchronously; onAck runs once with the broker outcome.
func (s *AtLeastOnceStrategy) Produce(ctx context.Context, out Outgoing, onAck AckFunc) {
	lease, err := s.lease(ctx, out.ConnectionID)
	if err != nil {
		onAck(err)
		return
	}
	rec := RecordMapper{}.ToKafka(out.Record)
	rec.Context = producers.PartitionModes{}.Tag(context.Background(), out.Partitioning)
	lease.Client.Produce(ctx, rec, func(_ *kgo.Record, err error) { onAck(err) })
}

// Commit commits marks on the consumer group synchronously.
func (s *AtLeastOnceStrategy) Commit(ctx context.Context, marks []Mark) error {
	if len(marks) == 0 {
		return nil
	}
	offsets := Marks(marks).Offsets()
	var commitErr error
	s.deps.Consumer.CommitOffsetsSync(ctx, offsets, func(_ *kgo.Client, _ *kmsg.OffsetCommitRequest, resp *kmsg.OffsetCommitResponse, err error) {
		commitErr = errors.Join(err, CommitResponse{}.Err(resp))
	})
	return commitErr
}

// Close flushes and releases every leased producer.
func (s *AtLeastOnceStrategy) Close(ctx context.Context) error {
	s.mu.Lock()
	leases := s.leases
	s.leases = map[string]*producers.Lease{}
	s.mu.Unlock()
	for _, l := range leases {
		s.deps.Producers.Release(ctx, l)
	}
	return nil
}

func (s *AtLeastOnceStrategy) lease(ctx context.Context, connectionID string) (*producers.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.leases[connectionID]; ok {
		return l, nil
	}
	spec, ok := s.deps.Connections[connectionID]
	if !ok {
		return nil, ErrUnknownConnection
	}
	l, err := s.deps.Producers.Acquire(ctx, spec, config.DeliveryModeAtLeastOnce, s.deps.Owner)
	if err != nil {
		return nil, err
	}
	s.leases[connectionID] = l
	return l, nil
}

// Marks is a list of committable positions.
type Marks []Mark

// Offsets groups the marks as franz-go commit offsets.
func (ms Marks) Offsets() map[string]map[int32]kgo.EpochOffset {
	offsets := map[string]map[int32]kgo.EpochOffset{}
	for _, m := range ms {
		if offsets[m.Topic] == nil {
			offsets[m.Topic] = map[int32]kgo.EpochOffset{}
		}
		offsets[m.Topic][m.Partition] = kgo.EpochOffset{Epoch: -1, Offset: m.Offset}
	}
	return offsets
}

// RecordMapper converts engine records to franz-go records.
type RecordMapper struct{}

// ToKafka copies key, value, headers, topic, source partition and timestamp.
func (RecordMapper) ToKafka(r engine.RawRecord) *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(r.Headers))
	for _, h := range r.Headers {
		headers = append(headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}
	return &kgo.Record{Key: r.Key, Value: r.Value, Headers: headers, Topic: r.Topic, Partition: r.Partition, Timestamp: r.Timestamp}
}

// FromKafka converts a consumed record.
func (RecordMapper) FromKafka(r *kgo.Record) engine.RawRecord {
	headers := make([]engine.Header, 0, len(r.Headers))
	for _, h := range r.Headers {
		headers = append(headers, engine.Header{Key: h.Key, Value: h.Value})
	}
	return engine.RawRecord{Key: r.Key, Value: r.Value, Headers: headers, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}
}

// CommitResponse extracts the first partition error of an offset commit response.
type CommitResponse struct{}

// Err returns the first non-zero partition error code as an error.
func (CommitResponse) Err(resp *kmsg.OffsetCommitResponse) error {
	if resp == nil {
		return nil
	}
	for _, t := range resp.Topics {
		for _, p := range t.Partitions {
			if err := kerr.ErrorForCode(p.ErrorCode); err != nil {
				return err
			}
		}
	}
	return nil
}
