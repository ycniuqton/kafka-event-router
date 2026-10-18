package runner

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ProbedPartition is the broker view of one partition assigned to this member; Lag is -1 while the group
// has no commit there (kadm would count from the log start, which is wrong for startFrom latest).
type ProbedPartition struct {
	End int64
	Lag int64
}

// ProbeValue is a sentinel of probed values.
type ProbeValue int64

// NoLag marks a probed partition without a group commit.
const NoLag ProbeValue = -1

// ProbedLag is the broker view of every partition assigned to this member.
type ProbedLag map[TopicPartition]ProbedPartition

// LagProbe asks the source cluster for the end offsets and group lag of this member's partitions, so the
// lag alert input keeps growing while fetching is paused (destination outage, backpressure) and is known
// before the runner's first commit.
type LagProbe struct {
	client *kgo.Client
	group  string
}

// NewLagProbe probes group on the consumer's own client (no extra connection).
func NewLagProbe(client *kgo.Client, group string) LagProbe {
	return LagProbe{client: client, group: group}
}

// Probe returns the partitions currently assigned to this member with their end offset and lag.
func (p LagProbe) Probe(ctx context.Context) (ProbedLag, error) {
	member, _ := p.client.GroupMetadata()
	lags, err := kadm.NewClient(p.client).Lag(ctx, p.group)
	if err != nil {
		return nil, err
	}
	out := ProbedLag{}
	for _, m := range lags[p.group].Lag.Sorted() {
		if m.Err != nil || m.Member == nil || m.Member.MemberID != member {
			continue
		}
		part := ProbedPartition{End: m.End.Offset, Lag: m.Lag}
		if m.Commit.At < 0 {
			part.Lag = int64(NoLag)
		}
		out[TopicPartition{Topic: m.Topic, Partition: m.Partition}] = part
	}
	return out, nil
}

// LagProbeLoop runs the probe every interval and hands each answer to apply.
type LagProbeLoop struct {
	Probe    LagProbe
	Interval time.Duration
	Apply    func(ProbedLag)
}

// Run probes until ctx ends; failed probes are skipped (the next tick retries).
func (l LagProbeLoop) Run(ctx context.Context) {
	t := time.NewTicker(l.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if probed, err := l.Probe.Probe(ctx); err == nil {
				l.Apply(probed)
			}
		}
	}
}
