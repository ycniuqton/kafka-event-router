package dlq

import (
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// DemoFailure is one kind of seeded failure: its error, the payload that caused it and how many to seed.
type DemoFailure struct {
	ErrorType record.ErrorType
	Message   string
	Stack     string
	OnStep    bool
	Value     DemoValue
	Count     int
}

// DemoFailures is the fixed failure mix of the demo workspace (12 dead letters over the last 2 hours).
var DemoFailures = []DemoFailure{
	{ErrorType: record.ErrorStep, Message: "Cannot read properties of null (reading 'reduce')", OnStep: true, Value: DemoNullItems{}, Count: 5,
		Stack: "TypeError: Cannot read properties of null (reading 'reduce')\n    at transform (step.js:2:29)\n    at sandbox.run (runtime:1:1)"},
	{ErrorType: record.ErrorOutputInvalid, Message: "Field amount: expected number, got string", Value: DemoStringAmount{}, Count: 3},
	{ErrorType: record.ErrorTimeout, Message: "JS step took 75 ms (limit 50 ms)", OnStep: true, Value: DemoIntact{}, Count: 2,
		Stack: "TimeoutError: execution exceeded 50 ms\n    at transform (step.js:4:5)"},
	{ErrorType: record.ErrorDecode, Message: "unexpected end of JSON input", Value: DemoTruncated{}, Count: 2},
}

// DemoSpread shapes the seeded set in time and source offsets.
type DemoSpread int64

const (
	DemoSpan          DemoSpread = DemoSpread(2 * time.Hour)
	DemoOrderBase     DemoSpread = 9000
	DemoOffsetBase    DemoSpread = 4200
	DemoOffsetStride  DemoSpread = 37
	DemoSeedGenerator DemoSpread = 20261014
)

// DemoDeadLetterSet builds the seeded dead letters, newest last, all failing within DemoSpan before now.
type DemoDeadLetterSet struct{}

// Records returns one record per DemoFailures entry and count.
func (DemoDeadLetterSet) Records(t DemoTarget, now time.Time) []record.Record {
	gen := kafka.NewDemoOrderGenerator(uint64(DemoSeedGenerator), 0)
	total := (DemoDeadLetterSet{}).total()
	step := time.Duration(DemoSpan) / time.Duration(total)
	out := make([]record.Record, 0, total)
	for _, f := range DemoFailures {
		for i := 0; i < f.Count; i++ {
			n := len(out)
			at := now.Add(-time.Duration(DemoSpan) + time.Duration(n)*step)
			out = append(out, (DemoDeadLetterSet{}).one(t, f, gen.Event(int(DemoOrderBase)+n, at), n, at))
		}
	}
	return out
}

func (DemoDeadLetterSet) total() int {
	n := 0
	for _, f := range DemoFailures {
		n += f.Count
	}
	return n
}

// one builds dead letter n from a demo order.
func (DemoDeadLetterSet) one(t DemoTarget, f DemoFailure, order kafka.DemoOrderEvent, n int, at time.Time) record.Record {
	failure := record.Failure{RouteID: t.RouteID, VersionNo: t.VersionNo, Branch: t.Branch, ErrorType: f.ErrorType,
		Message: f.Message, Stack: f.Stack, FailedAt: at.UTC()}
	if f.OnStep {
		failure.Step = t.Step
	}
	partition := int32(n) % int32(kafka.DemoPartitions)
	return record.Record{
		Key: []byte(order.OrderID), Value: f.Value.Bytes(order), Failure: failure,
		Headers: []engine.Header{{Key: string(kafka.DemoHeaderSource), Value: []byte(kafka.DemoSource)}, {Key: string(kafka.DemoHeaderEventType), Value: []byte(order.EventType)}},
		Source:  record.SourcePosition{Topic: t.SourceTopic, Partition: partition, Offset: int64(DemoOffsetBase) + int64(n)*int64(DemoOffsetStride)},
	}
}
