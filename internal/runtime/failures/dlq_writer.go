package failures

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// RecordEncoder turns a dead letter into Kafka bytes (dead-letters' record.Codec).
type RecordEncoder interface {
	Encode(r record.Record) engine.RawRecord
}

// Producer is the delivery surface the writer produces through.
type Producer interface {
	Produce(ctx context.Context, out delivery.Outgoing, onAck delivery.AckFunc)
}

// DLQWriter writes one failed record to the route's DLQ topic.
type DLQWriter interface {
	Write(ctx context.Context, f Failure, onAck delivery.AckFunc)
}

// DLQTarget is where a route's dead letters go.
type DLQTarget struct {
	ConnectionID string
	Topic        string
}

// RouteIdentity is the route version written into router.* headers.
type RouteIdentity struct {
	RouteID   string
	VersionNo int
}

// categoryErrorTypes maps runtime categories to DLQ error types for failures without an engine type.
var categoryErrorTypes = map[domain.ErrorCategory]record.ErrorType{
	domain.CategoryDecode:        record.ErrorDecode,
	domain.CategoryStep:          record.ErrorStep,
	domain.CategoryStepTimeout:   record.ErrorTimeout,
	domain.CategoryOutputInvalid: record.ErrorOutputInvalid,
}

// KafkaDLQWriter builds the record contract and produces it; failed DLQ produces are destination
// transient and retried with backoff until acked or the runner stops.
type KafkaDLQWriter struct {
	Target   DLQTarget
	Route    RouteIdentity
	Producer Producer
	Encoder  RecordEncoder
	Retry    retry.Policy
	Now      func() time.Time
}

// Write implements DLQWriter.
func (w KafkaDLQWriter) Write(ctx context.Context, f Failure, onAck delivery.AckFunc) {
	out := delivery.Outgoing{Record: w.Encoder.Encode(w.recordOf(f)), ConnectionID: w.Target.ConnectionID, Partitioning: config.PartitioningModeSameAsSource}
	out.Record.Topic = w.Target.Topic
	out.Record.Partition = f.Source.Partition
	out.Record.Timestamp = f.Source.Timestamp
	w.produce(ctx, out, 0, onAck)
}

// produce sends out and re-sends it after backoff on failure.
func (w KafkaDLQWriter) produce(ctx context.Context, out delivery.Outgoing, attempt int, onAck delivery.AckFunc) {
	w.Producer.Produce(ctx, out, func(err error) {
		if err == nil || ctx.Err() != nil {
			onAck(err)
			return
		}
		go func() {
			if werr := w.Retry.Wait(ctx, attempt); werr != nil {
				onAck(werr)
				return
			}
			w.produce(ctx, out, attempt+1, onAck)
		}()
	})
}

// recordOf builds the dead-letters record of f.
func (w KafkaDLQWriter) recordOf(f Failure) record.Record {
	return record.Record{
		Key: f.Source.Key, Value: f.Source.Value, Headers: f.Source.Headers,
		Failure: record.Failure{RouteID: w.Route.RouteID, VersionNo: w.Route.VersionNo, Branch: f.Branch(), Step: f.Step(),
			ErrorType: w.errorType(f), Message: f.Message(), Stack: f.Stack(), FailedAt: w.Now()},
		Source: record.SourcePosition{Topic: f.Source.Topic, Partition: f.Source.Partition, Offset: f.Source.Offset},
	}
}

func (KafkaDLQWriter) errorType(f Failure) record.ErrorType {
	return ErrorTypes{}.Of(f.Category)
}

// ErrorTypes maps runtime categories to the dead-letters error type vocabulary.
type ErrorTypes struct{}

// Of returns the DLQ error type of a category (step_error when unmapped).
func (ErrorTypes) Of(c domain.ErrorCategory) record.ErrorType {
	if t, ok := categoryErrorTypes[c]; ok {
		return t
	}
	return record.ErrorStep
}

// DLQTargetResolver picks errorPolicy.dlqConnectionId, else the configured default (source or first destination).
type DLQTargetResolver struct {
	Default domain.DLQTargetKind
}

// Resolve returns the DLQ connection and topic of cfg.
func (r DLQTargetResolver) Resolve(cfg *config.RouteConfig) DLQTarget {
	t := DLQTarget{ConnectionID: cfg.DLQConnection(), Topic: domain.TopicNaming{}.DLQDefault(cfg.Name)}
	if cfg.ErrorPolicy != nil && cfg.ErrorPolicy.DLQTopic != nil && *cfg.ErrorPolicy.DLQTopic != "" {
		t.Topic = *cfg.ErrorPolicy.DLQTopic
	}
	explicit := cfg.ErrorPolicy != nil && cfg.ErrorPolicy.DLQConnectionID != nil
	if !explicit && r.Default == domain.DLQTargetDestination && len(cfg.Branches) > 0 {
		t.ConnectionID = cfg.Branches[0].Destination.ConnectionID
	}
	return t
}
