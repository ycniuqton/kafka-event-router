package audit_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/config"
)

// chanSink delivers into a channel; block makes Deliver wait until released.
type chanSink struct {
	kind  audit.SinkKind
	got   chan audit.StreamRecord
	block chan struct{}
}

func (s *chanSink) Kind() audit.SinkKind { return s.kind }
func (s *chanSink) Deliver(ctx context.Context, rec audit.StreamRecord) error {
	if s.block != nil {
		<-s.block
	}
	s.got <- rec
	return nil
}

type countObserver struct {
	mu                         sync.Mutex
	delivered, dropped, failed int
}

func (o *countObserver) Delivered(audit.SinkKind) { o.mu.Lock(); o.delivered++; o.mu.Unlock() }
func (o *countObserver) Dropped(audit.SinkKind)   { o.mu.Lock(); o.dropped++; o.mu.Unlock() }
func (o *countObserver) Failed(audit.SinkKind)    { o.mu.Lock(); o.failed++; o.mu.Unlock() }

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestStreamFansOutToEverySink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := audit.NewStream(ctx, 4, discardLog(), nil)
	a, b := &chanSink{kind: "a", got: make(chan audit.StreamRecord, 1)}, &chanSink{kind: "b", got: make(chan audit.StreamRecord, 1)}
	stream.Add(a)
	stream.Add(b)
	stream.Publish(audit.StreamRecord{ID: 7, Action: audit.ActionAuthLogin})
	for _, s := range []*chanSink{a, b} {
		select {
		case rec := <-s.got:
			if rec.ID != 7 {
				t.Fatalf("%s got %+v", s.kind, rec)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s not delivered", s.kind)
		}
	}
	if kinds := stream.Sinks(); len(kinds) != 2 {
		t.Fatalf("sinks: %v", kinds)
	}
}

func TestStreamDropsWhenSinkBufferFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obs := &countObserver{}
	stream := audit.NewStream(ctx, 1, discardLog(), obs)
	slow := &chanSink{kind: "slow", got: make(chan audit.StreamRecord, 10), block: make(chan struct{})}
	stream.Add(slow)
	for i := range 5 {
		stream.Publish(audit.StreamRecord{ID: int64(i)})
	}
	close(slow.block)
	time.Sleep(100 * time.Millisecond)
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.dropped < 3 || obs.dropped+obs.delivered != 5 {
		t.Fatalf("publish must never block; dropped=%d delivered=%d", obs.dropped, obs.delivered)
	}
}

func TestSinkRegistryBuildsConfiguredSinks(t *testing.T) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(buf, nil))
	sinks, err := audit.NewSinkRegistry().Build(config.AuditStreamConfig{Sinks: []config.AuditStreamKind{config.AuditStreamLog}}, log)
	if err != nil || len(sinks) != 1 || sinks[0].Kind() != config.AuditStreamLog {
		t.Fatalf("build: %v %v", sinks, err)
	}
	if err := sinks[0].Deliver(context.Background(), audit.StreamRecord{ID: 9, Action: audit.ActionAuthLogin}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"msg":"audit.event"`) || !strings.Contains(buf.String(), `"action":"auth.login"`) {
		t.Fatalf("log line: %s", buf.String())
	}
	if _, err := audit.NewSinkRegistry().Build(config.AuditStreamConfig{Sinks: []config.AuditStreamKind{"kafka"}}, log); err == nil {
		t.Fatal("unknown sink kind accepted")
	}
}
