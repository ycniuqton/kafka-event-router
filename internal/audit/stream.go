package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/config"
)

// SinkKind names a real-time audit sink implementation, selected by ROUTER_AUDIT_STREAM_SINKS.
type SinkKind = config.AuditStreamKind

// StreamRecord is one committed audit_log row as delivered to sinks (diff already redacted).
type StreamRecord struct {
	ID          int64           `json:"id"`
	At          time.Time       `json:"at"`
	Action      Action          `json:"action"`
	ObjectType  ObjectType      `json:"objectType"`
	ObjectID    string          `json:"objectId,omitempty"`
	ObjectLabel string          `json:"objectLabel,omitempty"`
	ActorKind   ActorKind       `json:"actorKind"`
	ActorID     *uuid.UUID      `json:"actorId,omitempty"`
	ActorName   string          `json:"actorName,omitempty"`
	TeamID      *uuid.UUID      `json:"teamId,omitempty"`
	IP          string          `json:"ip,omitempty"`
	RequestID   string          `json:"requestId,omitempty"`
	Sensitive   bool            `json:"sensitive"`
	Diff        json.RawMessage `json:"diff,omitempty"`
}

// NewStreamRecord combines a written entry with its database id and timestamp.
func NewStreamRecord(id int64, at time.Time, e Entry) StreamRecord {
	return StreamRecord{ID: id, At: at, Action: e.Action, ObjectType: e.Object, ObjectID: e.ObjectID, ObjectLabel: e.ObjectLabel,
		ActorKind: e.Actor.Kind, ActorID: e.Actor.UserID, ActorName: e.Actor.Name, TeamID: e.TeamID, IP: e.IP,
		RequestID: e.RequestID, Sensitive: e.Sensitive, Diff: e.Diff}
}

// Sink is the base of every real-time audit destination (log shipping, SIEM webhook, syslog, …).
type Sink interface {
	Kind() SinkKind
	Deliver(ctx context.Context, rec StreamRecord) error
}

// StreamPublisher is what the recorder fans committed entries out to.
type StreamPublisher interface {
	Publish(rec StreamRecord)
}

// SinkRegistrar lets features (access-control's SIEM export) attach their own sinks at wiring time.
type SinkRegistrar interface {
	Add(s Sink)
}

// StreamObserver counts deliveries per sink (Prometheus in production, nothing in tests).
type StreamObserver interface {
	Delivered(kind SinkKind)
	Dropped(kind SinkKind)
	Failed(kind SinkKind)
}

// StreamLog names the stream's log messages and attributes.
type StreamLog string

const (
	LogStreamDropped StreamLog = "audit stream: sink buffer full, entry dropped"
	LogStreamFailed  StreamLog = "audit stream: delivery failed"
	LogAttrSink      StreamLog = "sink"
	LogAttrAuditID   StreamLog = "audit_id"
	LogAttrStreamErr StreamLog = "error"
)

// Stream is the sink manager: every sink gets its own bounded queue and worker, so a slow SIEM never
// blocks requests; a full queue drops (counted and logged) instead of applying back-pressure.
type Stream struct {
	ctx      context.Context
	buffer   int
	log      *slog.Logger
	observer StreamObserver
	mu       sync.RWMutex
	lanes    []*sinkLane
}

// NewStream builds a stream whose workers live as long as ctx.
func NewStream(ctx context.Context, buffer int, log *slog.Logger, observer StreamObserver) *Stream {
	if observer == nil {
		observer = NoStreamObserver{}
	}
	return &Stream{ctx: ctx, buffer: max(buffer, 1), log: log, observer: observer}
}

// Add attaches a sink and starts its worker.
func (s *Stream) Add(sink Sink) {
	lane := &sinkLane{sink: sink, queue: make(chan StreamRecord, s.buffer)}
	s.mu.Lock()
	s.lanes = append(s.lanes, lane)
	s.mu.Unlock()
	go lane.run(s.ctx, s)
}

// Sinks lists the attached sink kinds.
func (s *Stream) Sinks() []SinkKind {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SinkKind, 0, len(s.lanes))
	for _, l := range s.lanes {
		out = append(out, l.sink.Kind())
	}
	return out
}

// Publish enqueues rec on every sink without blocking.
func (s *Stream) Publish(rec StreamRecord) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.lanes {
		l.offer(rec, s)
	}
}

// sinkLane is one sink's queue and drop counter.
type sinkLane struct {
	sink    Sink
	queue   chan StreamRecord
	dropped atomic.Int64
}

func (l *sinkLane) offer(rec StreamRecord, s *Stream) {
	select {
	case l.queue <- rec:
	default:
		l.dropped.Add(1)
		s.observer.Dropped(l.sink.Kind())
		s.log.Warn(string(LogStreamDropped), string(LogAttrSink), l.sink.Kind(), string(LogAttrAuditID), rec.ID)
	}
}

func (l *sinkLane) run(ctx context.Context, s *Stream) {
	for {
		select {
		case <-ctx.Done():
			return
		case rec := <-l.queue:
			l.deliver(ctx, rec, s)
		}
	}
}

func (l *sinkLane) deliver(ctx context.Context, rec StreamRecord, s *Stream) {
	if err := l.sink.Deliver(ctx, rec); err != nil {
		s.observer.Failed(l.sink.Kind())
		s.log.Warn(string(LogStreamFailed), string(LogAttrSink), l.sink.Kind(), string(LogAttrAuditID), rec.ID,
			string(LogAttrStreamErr), err)
		return
	}
	s.observer.Delivered(l.sink.Kind())
}

// NoStreamObserver discards delivery counts.
type NoStreamObserver struct{}

// Delivered implements StreamObserver.
func (NoStreamObserver) Delivered(SinkKind) {}

// Dropped implements StreamObserver.
func (NoStreamObserver) Dropped(SinkKind) {}

// Failed implements StreamObserver.
func (NoStreamObserver) Failed(SinkKind) {}

// NoStream is the publisher used before (or without) a configured stream.
type NoStream struct{}

// Publish implements StreamPublisher.
func (NoStream) Publish(StreamRecord) {}
