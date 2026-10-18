package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// HealthPath is served by the running feed and probed by `demoproducer health`.
const HealthPath = "/healthz"

// FeedCommand runs every configured stream until the context ends.
type FeedCommand struct{}

// Run opens one client per cluster, makes sure the topics exist, then paces every stream.
func (FeedCommand) Run(ctx context.Context, cfg FeedConfig) error {
	clients, err := ClusterClients{}.Open(cfg.Clusters)
	if err != nil {
		return err
	}
	defer clients.Close()
	beat := &Heartbeat{}
	go HealthServer{Beat: beat, Addr: cfg.HealthAddr}.Serve(ctx)
	TopicEnsurer{Clients: clients, Partitions: cfg.Partitions}.Ensure(ctx, cfg.Streams)
	var wg sync.WaitGroup
	for i, spec := range cfg.Streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			StreamLoop{Spec: spec, Client: clients[spec.Cluster], Gen: Generators[spec.Generator], Cfg: cfg, Beat: beat, Seed: uint64(i + 1)}.Run(ctx)
		}()
	}
	log.Printf("demoproducer: %d streams at %.2f ev/s each", len(cfg.Streams), cfg.Rate)
	wg.Wait()
	return nil
}

// ClusterClients are the producer clients by cluster alias.
type ClusterClients map[string]*kgo.Client

// Open creates one client per cluster (idempotent producer, acks=all).
func (ClusterClients) Open(clusters map[string][]string) (ClusterClients, error) {
	out := ClusterClients{}
	for name, brokers := range clusters {
		cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ClientID("demo-producer"), kgo.ProducerLinger(0))
		if err != nil {
			out.Close()
			return nil, fmt.Errorf("cluster %s: %w", name, err)
		}
		out[name] = cl
	}
	return out, nil
}

// Close flushes and closes every client.
func (c ClusterClients) Close() {
	for _, cl := range c {
		cl.Close()
	}
}

// TopicEnsurer creates missing demo topics with the configured partition count (before auto-creation could).
type TopicEnsurer struct {
	Clients    ClusterClients
	Partitions int32
}

// Ensure retries until every topic exists or the context ends.
func (t TopicEnsurer) Ensure(ctx context.Context, streams []StreamSpec) {
	for _, s := range streams {
		for ctx.Err() == nil {
			err := t.create(ctx, s)
			if err == nil {
				break
			}
			log.Printf("demoproducer: ensure %s/%s: %v (retrying)", s.Cluster, s.Topic, err)
			SleepCtx(ctx, FeedDefaults.RetryEvery)
		}
	}
}

func (t TopicEnsurer) create(ctx context.Context, s StreamSpec) error {
	res, err := kadm.NewClient(t.Clients[s.Cluster]).CreateTopic(ctx, t.Partitions, -1, nil, s.Topic)
	if err == nil {
		err = res.Err
	}
	if errors.Is(err, kerr.TopicAlreadyExists) {
		return nil
	}
	return err
}

// StreamLoop produces one event of a generator per interval to one topic.
type StreamLoop struct {
	Spec   StreamSpec
	Client *kgo.Client
	Gen    EventGenerator
	Cfg    FeedConfig
	Beat   *Heartbeat
	Seed   uint64
}

// Run ticks until the context ends; produce errors are logged and the loop continues.
func (l StreamLoop) Run(ctx context.Context) {
	rnd := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), l.Seed))
	ticker := time.NewTicker(l.Cfg.Interval())
	defer ticker.Stop()
	seq := time.Now().Unix()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			l.produce(ctx, l.Gen.Next(seq, rnd, l.Cfg.BadRatio))
		}
	}
}

func (l StreamLoop) produce(ctx context.Context, ev Event) {
	value, err := json.Marshal(ev.Value)
	if err != nil {
		log.Printf("demoproducer: encode: %v", err)
		return
	}
	rec := &kgo.Record{Topic: l.Spec.Topic, Key: []byte(ev.Key), Value: value}
	l.Client.Produce(ctx, rec, func(_ *kgo.Record, err error) {
		if err != nil {
			log.Printf("demoproducer: %s/%s: %v", l.Spec.Cluster, l.Spec.Topic, err)
			return
		}
		l.Beat.Touch()
	})
}

// Heartbeat remembers when a record was last acknowledged.
type Heartbeat struct {
	lastNs atomic.Int64
}

// Touch records an acknowledgement now.
func (h *Heartbeat) Touch() { h.lastNs.Store(time.Now().UnixNano()) }

// Fresh reports whether an acknowledgement happened within maxAge.
func (h *Heartbeat) Fresh(maxAge time.Duration) bool {
	return time.Since(time.Unix(0, h.lastNs.Load())) < maxAge
}

// HealthServer answers 200 on HealthPath while records keep being acknowledged.
type HealthServer struct {
	Beat *Heartbeat
	Addr string
}

// Serve runs until the context ends.
func (s HealthServer) Serve(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc(HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		if !s.Beat.Fresh(FeedDefaults.StaleAfter) {
			http.Error(w, "no acknowledged record recently", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: s.Addr, Handler: mux, ReadHeaderTimeout: FeedDefaults.RetryEvery}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("demoproducer: health server: %v", err)
	}
}

// HealthCommand probes the running feed's health endpoint (exit 1 when stale or unreachable).
type HealthCommand struct{}

// Run performs one GET with a short timeout.
func (HealthCommand) Run(ctx context.Context, cfg FeedConfig) error {
	ctx, cancel := context.WithTimeout(ctx, FeedDefaults.RetryEvery)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+cfg.HealthAddr+HealthPath, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("health: %s", res.Status)
	}
	return nil
}

// SleepCtx waits d or until the context ends.
func SleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
