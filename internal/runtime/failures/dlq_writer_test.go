package failures

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/sandbox"
)

var update = flag.Bool("update", false, "rewrite golden files")

type captureProducer struct {
	outs  []delivery.Outgoing
	fails int
}

func (p *captureProducer) Produce(_ context.Context, out delivery.Outgoing, onAck delivery.AckFunc) {
	p.outs = append(p.outs, out)
	if p.fails > 0 {
		p.fails--
		onAck(context.DeadlineExceeded)
		return
	}
	onAck(nil)
}

func TestDLQWriterHeadersGolden(t *testing.T) {
	prod := &captureProducer{}
	w := KafkaDLQWriter{Target: DLQTarget{ConnectionID: "c1", Topic: "router.dlq.orders"}, Route: RouteIdentity{RouteID: "r1", VersionNo: 3},
		Producer: prod, Encoder: record.Codec{}, Retry: retry.DefaultPolicy(),
		Now: func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }}
	src := engine.RawRecord{Key: []byte("k"), Value: []byte("{bad"), Topic: "orders", Partition: 2, Offset: 41,
		Headers: []engine.Header{{Key: "trace", Value: []byte("abc")}, {Key: "router.version", Value: []byte("spoofed")}}}
	f := Failure{Source: src, Err: &engine.ProcessError{Type: engine.ErrDecode, Branch: "main", StepID: "s1", Cause: context.Canceled}, Category: domain.CategoryDecode}
	var ackErr error = context.Canceled
	w.Write(context.Background(), f, func(err error) { ackErr = err })
	if ackErr != nil || len(prod.outs) != 1 {
		t.Fatalf("ack %v outs %d", ackErr, len(prod.outs))
	}
	out := prod.outs[0]
	if out.Record.Topic != "router.dlq.orders" || out.Record.Partition != 2 || string(out.Record.Value) != "{bad" || out.ConnectionID != "c1" {
		t.Fatalf("record %+v", out)
	}
	var b strings.Builder
	for _, h := range out.Record.Headers {
		b.WriteString(h.Key + "=" + string(h.Value) + "\n")
	}
	golden := filepath.Join("testdata", "dlq_headers.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Fatalf("headers differ from golden:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestDLQWriterRetriesFailedProduce(t *testing.T) {
	prod := &captureProducer{fails: 2}
	w := KafkaDLQWriter{Target: DLQTarget{ConnectionID: "c1", Topic: "t"}, Producer: prod, Encoder: record.Codec{},
		Retry: retry.Policy{Min: time.Millisecond, Max: 2 * time.Millisecond, Factor: 2}, Now: time.Now}
	done := make(chan error, 1)
	w.Write(context.Background(), Failure{Category: domain.CategoryStep}, func(err error) { done <- err })
	select {
	case err := <-done:
		if err != nil || len(prod.outs) != 3 {
			t.Fatalf("err %v attempts %d", err, len(prod.outs))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no ack")
	}
}

func TestDLQWriterKeepsTheJavaScriptStack(t *testing.T) {
	prod := &captureProducer{}
	w := KafkaDLQWriter{Target: DLQTarget{ConnectionID: "c1", Topic: "router.dlq.orders"}, Route: RouteIdentity{RouteID: "r1", VersionNo: 1},
		Producer: prod, Encoder: record.Codec{}, Retry: retry.DefaultPolicy(), Now: time.Now}
	jsErr := &sandbox.Error{Kind: sandbox.ErrorRuntime, Message: "Error: bad event 1", Stack: "    at transform (step.js:1:80)\n"}
	f := Failure{Source: engine.RawRecord{Value: []byte(`{"bad":true}`)}, Err: &engine.ProcessError{Type: engine.ErrStep, Branch: "main", StepID: "js", Cause: jsErr}, Category: domain.CategoryStep}
	w.Write(context.Background(), f, func(error) {})
	if len(prod.outs) != 1 {
		t.Fatalf("outs %d", len(prod.outs))
	}
	for _, h := range prod.outs[0].Record.Headers {
		if h.Key == string(record.HeaderStack) {
			if string(h.Value) != jsErr.Stack {
				t.Fatalf("stack header %q, want %q", h.Value, jsErr.Stack)
			}
			return
		}
	}
	t.Fatal("no router.stack header")
}
