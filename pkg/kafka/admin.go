package kafka

import (
	"context"
	"errors"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TopicInfo describes one topic of a cluster.
type TopicInfo struct {
	Name              string `json:"name"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	Internal          bool   `json:"internal"`
}

// TopicConfigKey names broker topic configs set at creation.
type TopicConfigKey string

const TopicRetentionMs TopicConfigKey = "retention.ms"

// TopicSpec is a create-topic request; ReplicationFactor -1 means the broker default.
type TopicSpec struct {
	Name              string
	Partitions        int32
	ReplicationFactor int16
	RetentionMs       int64
	Configs           map[string]string
}

// Admin is the cluster administration surface used by the API, preview and workers.
type Admin interface {
	Ping(ctx context.Context) error
	ListTopics(ctx context.Context) ([]TopicInfo, error)
	DescribeTopics(ctx context.Context, names ...string) ([]TopicInfo, error)
	EndOffsets(ctx context.Context, topics ...string) (map[string]int64, error)
	CreateTopic(ctx context.Context, spec TopicSpec) (TopicInfo, error)
	DescribeGroup(ctx context.Context, group string) (GroupInfo, error)
	OffsetsForTimes(ctx context.Context, at time.Time, topics ...string) (PartitionOffsets, error)
	CommitOffsets(ctx context.Context, group string, offsets PartitionOffsets) error
}

// FranzAdmin implements Admin on kadm.
type FranzAdmin struct {
	raw *kgo.Client
	adm *kadm.Client
}

// NewFranzAdmin wraps a franz-go client (the caller keeps ownership).
func NewFranzAdmin(cl *kgo.Client) *FranzAdmin {
	return &FranzAdmin{raw: cl, adm: kadm.NewClient(cl)}
}

// Ping sends ApiVersions to any broker (including the SASL/TLS handshake).
func (a *FranzAdmin) Ping(ctx context.Context) error {
	return a.raw.Ping(ctx)
}

// ListTopics returns every topic (internal included, flagged) sorted by name.
func (a *FranzAdmin) ListTopics(ctx context.Context) ([]TopicInfo, error) {
	return a.describe(ctx)
}

// DescribeTopics returns the named topics; a missing topic yields its broker error.
func (a *FranzAdmin) DescribeTopics(ctx context.Context, names ...string) ([]TopicInfo, error) {
	return a.describe(ctx, names...)
}

// EndOffsets returns the sum of partition high watermarks per topic.
func (a *FranzAdmin) EndOffsets(ctx context.Context, topics ...string) (map[string]int64, error) {
	listed, err := a.adm.ListEndOffsets(ctx, topics...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(listed))
	listed.Each(func(o kadm.ListedOffset) {
		if o.Err == nil && o.Offset > 0 {
			out[o.Topic] += o.Offset
		}
	})
	return out, nil
}

// CreateTopic creates one topic with optional retention and configs.
func (a *FranzAdmin) CreateTopic(ctx context.Context, spec TopicSpec) (TopicInfo, error) {
	configs := map[string]*string{}
	for k, v := range spec.Configs {
		configs[k] = &v
	}
	if spec.RetentionMs > 0 {
		retention := strconv.FormatInt(spec.RetentionMs, 10)
		configs[string(TopicRetentionMs)] = &retention
	}
	resp, err := a.adm.CreateTopic(ctx, spec.Partitions, spec.ReplicationFactor, configs, spec.Name)
	if err != nil {
		return TopicInfo{}, err
	}
	return TopicInfo{Name: spec.Name, Partitions: int(resp.NumPartitions), ReplicationFactor: int(resp.ReplicationFactor)}, nil
}

func (a *FranzAdmin) describe(ctx context.Context, names ...string) ([]TopicInfo, error) {
	details, err := a.adm.ListTopicsWithInternal(ctx, names...)
	if err != nil {
		return nil, err
	}
	out := make([]TopicInfo, 0, len(details))
	for _, name := range details.Names() {
		d := details[name]
		if d.Err != nil {
			return nil, d.Err
		}
		out = append(out, TopicInfo{Name: name, Partitions: len(d.Partitions), ReplicationFactor: d.Partitions.NumReplicas(), Internal: d.IsInternal})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Network names the dial network of seed probes.
type Network string

const NetworkTCP Network = "tcp"

// ErrNoSeeds is returned when a spec has no bootstrap server.
var ErrNoSeeds = errors.New("kafka: no bootstrap servers")

// SeedDialer checks raw TCP reachability of the bootstrap servers (no TLS, no SASL).
type SeedDialer struct {
	Timeout time.Duration
}

// Reach succeeds when at least one seed accepts a TCP connection; otherwise it returns the last error.
func (d SeedDialer) Reach(ctx context.Context, seeds []string) error {
	lastErr := ErrNoSeeds
	dialer := net.Dialer{Timeout: d.timeout()}
	for _, seed := range seeds {
		conn, err := dialer.DialContext(ctx, string(NetworkTCP), seed)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func (d SeedDialer) timeout() time.Duration {
	if d.Timeout <= 0 {
		return time.Duration(DialTimeout)
	}
	return d.Timeout
}
