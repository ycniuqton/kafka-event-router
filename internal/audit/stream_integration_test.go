package audit_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eventrouter/router/internal/audit"
)

// memPublisher records published stream records.
type memPublisher struct {
	mu   sync.Mutex
	recs []audit.StreamRecord
}

func (p *memPublisher) Publish(rec audit.StreamRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recs = append(p.recs, rec)
}

func (p *memPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.recs)
}

// TestRecorderStreamsOnlyCommittedEntries: rolled-back entries never reach sinks; committed ones carry the row id.
func TestRecorderStreamsOnlyCommittedEntries(t *testing.T) {
	f := newAuditFixture(t)
	pub := &memPublisher{}
	f.recorder.StreamTo(pub)
	ctx := context.Background()
	entry := audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionAuthLogin, Object: audit.ObjectSession, RequestID: "req-stream"}
	err := f.txm.WithinTx(ctx, func(ctx context.Context) error {
		if err := f.recorder.Record(ctx, entry); err != nil {
			return err
		}
		if pub.count() != 0 {
			t.Fatal("published before commit")
		}
		return errors.New("rollback")
	})
	if err == nil || pub.count() != 0 {
		t.Fatalf("rolled back entry streamed: %v %d", err, pub.count())
	}
	err = f.txm.WithinTx(ctx, func(ctx context.Context) error { return f.recorder.Record(ctx, entry) })
	if err != nil || pub.count() != 1 || pub.recs[0].ID == 0 || pub.recs[0].At.IsZero() || pub.recs[0].RequestID != "req-stream" {
		t.Fatalf("committed entry: %v %+v", err, pub.recs)
	}
	if err := f.recorder.Record(ctx, entry); err != nil || pub.count() != 2 {
		t.Fatalf("entry outside tx: %v %d", err, pub.count())
	}
}
