package kafkaops

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// OffsetRetry bounds the retries of a listing that hits a retriable broker error (a freshly created topic
// without a leader yet answers NOT_LEADER_FOR_PARTITION / LEADER_NOT_AVAILABLE).
type OffsetRetry struct {
	Attempts int
	Wait     time.Duration
}

// DefaultOffsetRetry waits up to ~30 s for new topics to get leaders.
var DefaultOffsetRetry = OffsetRetry{Attempts: 30, Wait: time.Second}

// StartPoint says where a measurer begins reading.
type StartPoint string

const (
	FromStart StartPoint = "start"
	FromEnd   StartPoint = "end"
)

// Offsets resolves exact per-partition start offsets, so no record produced after "ready" can be missed;
// retriable broker errors (topics still electing leaders) are retried.
func (a *Admin) Offsets(ctx context.Context, from StartPoint, committedOnly bool, topics ...string) (map[string]map[int32]kgo.Offset, error) {
	var err error
	for i := 0; i < DefaultOffsetRetry.Attempts; i++ {
		var out map[string]map[int32]kgo.Offset
		if out, err = a.offsetsOnce(ctx, from, committedOnly, topics...); err == nil || !kerr.IsRetriable(err) {
			return out, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(DefaultOffsetRetry.Wait):
		}
	}
	return nil, err
}

// offsetsOnce lists the offsets once.
func (a *Admin) offsetsOnce(ctx context.Context, from StartPoint, committedOnly bool, topics ...string) (map[string]map[int32]kgo.Offset, error) {
	var (
		listed interface {
			KOffsets() map[string]map[int32]kgo.Offset
			Error() error
		}
		err error
	)
	switch {
	case from == FromStart:
		listed, err = a.adm.ListStartOffsets(ctx, topics...)
	case committedOnly:
		listed, err = a.adm.ListCommittedOffsets(ctx, topics...)
	default:
		listed, err = a.adm.ListEndOffsets(ctx, topics...)
	}
	if err != nil {
		return nil, err
	}
	if err := listed.Error(); err != nil {
		return nil, fmt.Errorf("list offsets: %w", err)
	}
	return listed.KOffsets(), nil
}

// GroupLag returns the total lag of each consumer group (missing/empty groups report the lag they can compute).
func (a *Admin) GroupLag(ctx context.Context, groups ...string) (map[string]int64, error) {
	lags, err := a.adm.Lag(ctx, groups...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(groups))
	for g, l := range lags {
		out[g] = l.Lag.Total()
	}
	return out, nil
}

// TopicEnds returns the summed end offset (record count for never-truncated topics) per topic.
func (a *Admin) TopicEnds(ctx context.Context, topics ...string) (map[string]int64, error) {
	ends, err := a.adm.ListEndOffsets(ctx, topics...)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for t, ps := range ends.Offsets() {
		for _, o := range ps {
			out[t] += o.At
		}
	}
	return out, nil
}
