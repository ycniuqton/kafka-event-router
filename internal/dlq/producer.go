package dlq

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// ErrPermanentDestination marks a destination error that retrying will not fix.
var ErrPermanentDestination = errors.New("destination rejected the record permanently")

// FranzProducer produces with one idempotent acks=all client per connection and bounded backoff retries.
type FranzProducer struct {
	clients ClientOpener
	mu      sync.Mutex
	open    map[string]*kgo.Client
}

// NewFranzProducer builds the producer.
func NewFranzProducer(clients ClientOpener) *FranzProducer {
	return &FranzProducer{clients: clients, open: map[string]*kgo.Client{}}
}

// Produce implements DestinationProducer.
func (p *FranzProducer) Produce(ctx context.Context, conn string, recs []engine.RawRecord) ([]Destination, error) {
	cl, err := p.client(ctx, conn)
	if err != nil {
		return nil, err
	}
	var last error
	for attempt := 0; attempt < int(ProduceAttempts); attempt++ {
		dests, err := p.once(ctx, cl, recs)
		if err == nil {
			return dests, nil
		}
		if (ProduceErrors{}).Permanent(err) {
			return nil, errors.Join(ErrPermanentDestination, err)
		}
		last = err
		if (Backoff{}).Wait(ctx, attempt) != nil {
			break
		}
	}
	return nil, last
}

func (p *FranzProducer) once(ctx context.Context, cl *kgo.Client, recs []engine.RawRecord) ([]Destination, error) {
	krs := make([]*kgo.Record, 0, len(recs))
	for _, r := range recs {
		hs := make([]kgo.RecordHeader, 0, len(r.Headers))
		for _, h := range r.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		krs = append(krs, &kgo.Record{Topic: r.Topic, Key: r.Key, Value: r.Value, Headers: hs})
	}
	results := cl.ProduceSync(ctx, krs...)
	if err := results.FirstErr(); err != nil {
		return nil, err
	}
	out := make([]Destination, 0, len(results))
	for _, res := range results {
		out = append(out, Destination{Topic: res.Record.Topic, Partition: res.Record.Partition, Offset: res.Record.Offset})
	}
	return out, nil
}

func (p *FranzProducer) client(ctx context.Context, conn string) (*kgo.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cl, ok := p.open[conn]; ok {
		return cl, nil
	}
	cl, err := p.clients.Open(ctx, conn, kafka.PurposeProducer, kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		return nil, err
	}
	p.open[conn] = cl
	return cl, nil
}

// Close closes every producer client.
func (p *FranzProducer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, cl := range p.open {
		cl.Close()
	}
	p.open = map[string]*kgo.Client{}
}

// ProduceErrors classifies produce errors.
type ProduceErrors struct{}

// Permanent reports a non-retriable broker error (authorization, too large, invalid record).
func (ProduceErrors) Permanent(err error) bool {
	var ke *kerr.Error
	return errors.As(err, &ke) && !ke.Retriable
}

// Backoff waits between produce attempts.
type Backoff struct{}

// Wait sleeps ProduceBackoff × (attempt+1) or returns ctx's error.
func (Backoff) Wait(ctx context.Context, attempt int) error {
	t := time.NewTimer(ProduceBackoff.Duration() * time.Duration(attempt+1))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
