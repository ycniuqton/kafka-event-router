package kafka

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

// DemoTopic names the topics of the demo cluster.
type DemoTopic string

const DemoOrderEvents DemoTopic = "order-events"

// DemoBrokerConfig names kfake broker configs set on demo clusters.
type DemoBrokerConfig string

const (
	DemoCleanerBackoffMs DemoBrokerConfig = "log.cleaner.backoff.ms"
	DemoLogRetentionMs   DemoBrokerConfig = "log.retention.ms"
)

// DemoSettings shape a demo cluster: topic layout, seeded backlog, live rate and memory bound (retention).
type DemoSettings struct {
	Partitions     int32
	Backlog        int
	BacklogSpan    time.Duration
	Interval       time.Duration
	Retention      time.Duration
	CleanerBackoff time.Duration
	Seed           uint64
}

// DemoDefaults are the default demo cluster numbers.
type DemoDefaults int64

const (
	DemoPartitions     DemoDefaults = 3
	DemoBacklog        DemoDefaults = 120
	DemoBacklogSpan    DemoDefaults = DemoDefaults(10 * time.Minute)
	DemoInterval       DemoDefaults = DemoDefaults(time.Second)
	DemoRetention      DemoDefaults = DemoDefaults(6 * time.Hour)
	DemoCleanerBackoff DemoDefaults = DemoDefaults(5 * time.Minute)
	DemoSeed           DemoDefaults = 20261014
	DemoStartTimeout   DemoDefaults = DemoDefaults(10 * time.Second)
)

// DefaultDemoSettings returns one event per second on 3 partitions, 120 seeded events over 10 minutes, 6 h retention.
func DefaultDemoSettings() DemoSettings {
	return DemoSettings{
		Partitions: int32(DemoPartitions), Backlog: int(DemoBacklog), BacklogSpan: time.Duration(DemoBacklogSpan),
		Interval: time.Duration(DemoInterval), Retention: time.Duration(DemoRetention),
		CleanerBackoff: time.Duration(DemoCleanerBackoff), Seed: uint64(DemoSeed),
	}
}

// DemoCluster is one running in-memory kfake cluster with its order-event generator.
type DemoCluster struct {
	fake     *kfake.Cluster
	producer *kgo.Client
	stop     context.CancelFunc
	done     sync.WaitGroup
}

// StartDemoCluster starts the cluster, creates `order-events`, seeds the backlog and starts the live feed.
func StartDemoCluster(ctx context.Context, s DemoSettings) (*DemoCluster, error) {
	fake, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.AllowAutoTopicCreation(), kfake.DefaultNumPartitions(int(s.Partitions)),
		kfake.BrokerConfigs(map[string]string{
			string(DemoCleanerBackoffMs): strconv.FormatInt(s.CleanerBackoff.Milliseconds(), 10),
			string(DemoLogRetentionMs):   strconv.FormatInt(s.Retention.Milliseconds(), 10),
		}))
	if err != nil {
		return nil, fmt.Errorf("kafka: start demo cluster: %w", err)
	}
	producer, err := kgo.NewClient(kgo.SeedBrokers(fake.ListenAddrs()...), kgo.DefaultProduceTopic(string(DemoOrderEvents)))
	if err != nil {
		fake.Close()
		return nil, fmt.Errorf("kafka: demo producer: %w", err)
	}
	c := &DemoCluster{fake: fake, producer: producer}
	if err := c.seed(ctx, s); err != nil {
		c.Close()
		return nil, err
	}
	feedCtx, stop := context.WithCancel(context.Background())
	c.stop = stop
	c.done.Add(1)
	go c.feed(feedCtx, s, NewDemoOrderGenerator(s.Seed, s.Backlog))
	return c, nil
}

// Seeds returns the cluster's listen addresses.
func (c *DemoCluster) Seeds() []string { return c.fake.ListenAddrs() }

// Close stops the feed and the cluster.
func (c *DemoCluster) Close() {
	if c.stop != nil {
		c.stop()
		c.done.Wait()
	}
	c.producer.Close()
	c.fake.Close()
}

// seed creates the topic with retention and produces the backlog spread over BacklogSpan.
func (c *DemoCluster) seed(ctx context.Context, s DemoSettings) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(DemoStartTimeout))
	defer cancel()
	spec := TopicSpec{Name: string(DemoOrderEvents), Partitions: s.Partitions, ReplicationFactor: -1, RetentionMs: s.Retention.Milliseconds()}
	if _, err := NewFranzAdmin(c.producer).CreateTopic(ctx, spec); err != nil {
		return fmt.Errorf("kafka: demo topic: %w", err)
	}
	gen := NewDemoOrderGenerator(s.Seed, 0)
	start, step := time.Now().Add(-s.BacklogSpan), s.BacklogSpan/time.Duration(max(s.Backlog, 1))
	records := make([]*kgo.Record, 0, s.Backlog)
	for i := 0; i < s.Backlog; i++ {
		records = append(records, gen.Next(start.Add(time.Duration(i)*step)))
	}
	if err := c.producer.ProduceSync(ctx, records...).FirstErr(); err != nil {
		return fmt.Errorf("kafka: demo backlog: %w", err)
	}
	return nil
}

// feed produces one order event per interval until stopped (errors are dropped: the demo is best effort).
func (c *DemoCluster) feed(ctx context.Context, s DemoSettings, gen *DemoOrderGenerator) {
	defer c.done.Done()
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.producer.Produce(ctx, gen.Next(now), nil)
		}
	}
}
