package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EnvKey names the environment variables the feed reads.
type EnvKey string

const (
	EnvClusters   EnvKey = "DEMO_CLUSTERS"
	EnvStreams    EnvKey = "DEMO_STREAMS"
	EnvRate       EnvKey = "DEMO_RATE"
	EnvBadRatio   EnvKey = "DEMO_BAD_RATIO"
	EnvPartitions EnvKey = "DEMO_PARTITIONS"
	EnvHealthAddr EnvKey = "DEMO_HEALTH_ADDR"
)

// FeedDefaults are used when a variable is unset.
var FeedDefaults = struct {
	Clusters, Streams, HealthAddr string
	Rate, BadRatio                float64
	Partitions                    int
	StaleAfter, RetryEvery        time.Duration
}{
	Clusters:   "a=kafka:19092;b=kafka-b:19092",
	Streams:    "orders@a/demo-orders,payments@b/demo-payments,raw@b/demo-raw,local@b/demo-b-in",
	HealthAddr: "127.0.0.1:8090",
	Rate:       1,
	BadRatio:   0.1,
	Partitions: 3,
	StaleAfter: 30 * time.Second,
	RetryEvery: 5 * time.Second,
}

// ListSeparators of the DEMO_CLUSTERS / DEMO_STREAMS syntax.
var ListSeparators = struct{ Clusters, Assign, Streams, At, Topic string }{Clusters: ";", Assign: "=", Streams: ",", At: "@", Topic: "/"}

// StreamSpec is one generator writing to a topic of a cluster: "<generator>@<cluster>/<topic>".
type StreamSpec struct {
	Generator, Cluster, Topic string
}

// FeedConfig is the whole feed configuration.
type FeedConfig struct {
	Clusters   map[string][]string
	Streams    []StreamSpec
	Rate       float64
	BadRatio   float64
	Partitions int32
	HealthAddr string
}

// Interval is the pause between two events of one stream.
func (c FeedConfig) Interval() time.Duration {
	return time.Duration(float64(time.Second) / c.Rate)
}

// FeedConfigFromEnv parses the DEMO_* variables (lookup returns "" when unset).
func FeedConfigFromEnv(lookup func(string) string) (FeedConfig, error) {
	env := EnvReader{lookup: lookup}
	cfg := FeedConfig{HealthAddr: env.str(EnvHealthAddr, FeedDefaults.HealthAddr)}
	var err error
	if cfg.Clusters, err = env.clusters(); err != nil {
		return cfg, err
	}
	if cfg.Streams, err = env.streams(cfg.Clusters); err != nil {
		return cfg, err
	}
	if cfg.Rate, err = env.float(EnvRate, FeedDefaults.Rate); err != nil || cfg.Rate <= 0 {
		return cfg, fmt.Errorf("%s must be a positive number", EnvRate)
	}
	if cfg.BadRatio, err = env.float(EnvBadRatio, FeedDefaults.BadRatio); err != nil {
		return cfg, err
	}
	partitions, err := env.float(EnvPartitions, float64(FeedDefaults.Partitions))
	cfg.Partitions = int32(partitions)
	return cfg, err
}

// EnvReader reads typed values with defaults.
type EnvReader struct {
	lookup func(string) string
}

func (e EnvReader) str(key EnvKey, def string) string {
	if v := strings.TrimSpace(e.lookup(string(key))); v != "" {
		return v
	}
	return def
}

func (e EnvReader) float(key EnvKey, def float64) (float64, error) {
	raw := e.str(key, "")
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def, fmt.Errorf("%s: %w", key, err)
	}
	return v, nil
}

// clusters parses "a=host:port,host:port;b=host:port".
func (e EnvReader) clusters() (map[string][]string, error) {
	out := map[string][]string{}
	for _, part := range strings.Split(e.str(EnvClusters, FeedDefaults.Clusters), ListSeparators.Clusters) {
		name, brokers, ok := strings.Cut(strings.TrimSpace(part), ListSeparators.Assign)
		if !ok || name == "" || brokers == "" {
			return nil, fmt.Errorf("%s: bad entry %q (want name=host:port)", EnvClusters, part)
		}
		out[name] = strings.Split(brokers, ListSeparators.Streams)
	}
	return out, nil
}

// streams parses "generator@cluster/topic,..." and checks the cluster and generator exist.
func (e EnvReader) streams(clusters map[string][]string) ([]StreamSpec, error) {
	var out []StreamSpec
	for _, part := range strings.Split(e.str(EnvStreams, FeedDefaults.Streams), ListSeparators.Streams) {
		gen, rest, okAt := strings.Cut(strings.TrimSpace(part), ListSeparators.At)
		cluster, topic, okTopic := strings.Cut(rest, ListSeparators.Topic)
		if !okAt || !okTopic || topic == "" {
			return nil, fmt.Errorf("%s: bad entry %q (want generator@cluster/topic)", EnvStreams, part)
		}
		if _, ok := clusters[cluster]; !ok {
			return nil, fmt.Errorf("%s: unknown cluster %q", EnvStreams, cluster)
		}
		if _, ok := Generators[gen]; !ok {
			return nil, fmt.Errorf("%s: unknown generator %q", EnvStreams, gen)
		}
		out = append(out, StreamSpec{Generator: gen, Cluster: cluster, Topic: topic})
	}
	return out, nil
}
