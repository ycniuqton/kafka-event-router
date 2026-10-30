package runner

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/retrytopic"
	"github.com/eventrouter/router/pkg/kafka"
)

// RetryNaming holds the suffixes of the retry consumer's group and producer owner.
type RetryNaming string

const (
	RetryGroupSuffix RetryNaming = ".retry"
	RetryOwnerSuffix RetryNaming = ".retry"
)

// RetryTopicPurpose labels tier topics created on runner start.
var RetryTopicPurpose = failures.TopicPurpose{Label: "Retry topic", Reason: domain.HaltDLQTopicUnavailable}

// RetryRuntime is a runner's retry consumer: its client, its own at-least-once strategy and the poll loop.
type RetryRuntime struct {
	client   *kgo.Client
	strategy delivery.Strategy
	consumer *retrytopic.Consumer
	done     chan struct{}
	started  bool
}

// retryRoute builds the retry policy of v (nil when errorPolicy.retry is not enabled).
func (r *RouteRunner) retryRoute(v CompiledVersion) *RetryRoute {
	if v.Config == nil {
		return nil
	}
	planner := retrytopic.NewPlanner(v.Config.ErrorPolicy)
	if !planner.Enabled() {
		return nil
	}
	now := r.deps.Now
	if now == nil {
		now = time.Now
	}
	target := r.deps.DLQTargets.Resolve(v.Config)
	scheduler := retrytopic.Scheduler{Planner: planner, RouteName: v.Config.Name, ConnectionID: target.ConnectionID, Now: now}
	return &RetryRoute{planner: planner, scheduler: scheduler, metrics: r.deps.Metrics.retry, labels: r.deps.Metrics.For(v).RetryLabels()}
}

// dlqWriter builds the DLQ writer of v producing through producer.
func (r *RouteRunner) dlqWriter(v CompiledVersion, producer failures.Producer) failures.DLQWriter {
	return failures.KafkaDLQWriter{Target: r.deps.DLQTargets.Resolve(v.Config), Route: failures.RouteIdentity{RouteID: v.Ref.RouteID, VersionNo: v.Ref.VersionNo},
		Producer: producer, Encoder: r.deps.Encoder, Retry: r.deps.Retry, Now: r.deps.Now}
}

// startRetryTopics ensures the tier topics and opens the retry consumer when v enables errorPolicy.retry.
func (r *RouteRunner) startRetryTopics(ctx context.Context, v CompiledVersion) error {
	if err := r.ensureRetryTiers(ctx, v); err != nil {
		return err
	}
	return r.openRetries(ctx, v)
}

// ensureRetryTiers creates the tier topics of an enabled retry policy on the DLQ connection.
func (r *RouteRunner) ensureRetryTiers(ctx context.Context, v CompiledVersion) error {
	rr := r.retryRoute(v)
	if rr == nil {
		return nil
	}
	admin, err := r.deps.Opener.Admin(ctx, v.Connections[rr.scheduler.ConnectionID])
	if err != nil {
		return err
	}
	defer admin.Close()
	for _, spec := range rr.scheduler.TopicSpecs(r.partitions) {
		if err := (failures.TopicEnsurer{}).EnsureSpec(ctx, admin.Admin(), spec, RetryTopicPurpose); err != nil {
			return err
		}
	}
	return nil
}

// openRetries opens the retry consumer group over every tier and its at-least-once strategy.
func (r *RouteRunner) openRetries(ctx context.Context, v CompiledVersion) error {
	rr := r.retryRoute(v)
	if rr == nil {
		return nil
	}
	consumer := &retrytopic.Consumer{Backoff: r.deps.Retry, Now: rr.scheduler.Now, Metrics: rr.metrics, RouteID: v.Ref.RouteID, Log: r.deps.Log}
	opts := append(consumer.Options(), kgo.ConsumerGroup(ConsumerGroups{}.Of(v)+string(RetryGroupSuffix)),
		kgo.ConsumeTopics(rr.scheduler.Topics()...), kgo.FetchMaxWait(time.Duration(SourceFetchMaxWait)))
	client, err := r.deps.Opener.Open(ctx, v.Connections[rr.scheduler.ConnectionID], kafka.PurposeConsumer, opts...)
	if err != nil {
		return err
	}
	strategy, err := delivery.AtLeastOnceFactory{}.New(ctx, delivery.Deps{Consumer: client, Producers: r.deps.Producers,
		Connections: v.Connections, Source: rr.scheduler.ConnectionID, Owner: v.Ref.RouteID + string(RetryOwnerSuffix)})
	if err != nil {
		client.Close()
		return err
	}
	consumer.Committer = strategy
	r.retries = &RetryRuntime{client: client, strategy: strategy, consumer: consumer, done: make(chan struct{})}
	return nil
}

// launchRetries starts the retry poll loop bound to the runner's poll context.
func (r *RouteRunner) launchRetries(ctx context.Context) {
	rt := r.retries
	if rt == nil {
		return
	}
	rt.consumer.Handler = &RetryHandler{flow: r.flow, strategy: rt.strategy, writers: r.dlqWriter}
	rt.started = true
	go func() {
		defer close(rt.done)
		rt.consumer.Run(ctx, rt.client)
	}()
}

// closeRetries waits for the retry loop (its context is already cancelled), then releases its clients.
func (r *RouteRunner) closeRetries(ctx context.Context) {
	rt := r.retries
	if rt == nil {
		return
	}
	if rt.started {
		select {
		case <-rt.done:
		case <-time.After(r.deps.Timings.Drain):
		}
	}
	_ = rt.strategy.Close(ctx)
	rt.client.Close()
}
