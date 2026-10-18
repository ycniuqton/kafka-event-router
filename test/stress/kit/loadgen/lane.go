package loadgen

import (
	"context"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/test/stress/kit/pacing"
	"github.com/eventrouter/router/test/stress/kit/payload"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// lane is one producing goroutine: it owns a disjoint set of streams, so per-stream sequence numbers stay gapless.
type lane struct {
	streams []string
	topics  []string
	seqs    []int64
	picker  payload.Picker
	tpl     *payload.Template
	rng     *rand.Rand
	bad     float64
	size    int
	owner   *Producer
}

func newLane(index int, streams []string, tpl *payload.Template, cfg ProduceConfig, owner *Producer) (*lane, error) {
	picker, err := payload.KeyManager{}.New(cfg.Keys, uint64(index)+1)
	if err != nil {
		return nil, err
	}
	topics := make([]string, len(streams))
	for i := range streams {
		topics[i] = cfg.Topics[(index+i*cfg.Lanes)%len(cfg.Topics)]
	}
	return &lane{
		streams: streams, topics: topics, seqs: make([]int64, len(streams)), picker: picker, tpl: tpl, bad: cfg.BadRatio,
		rng: rand.New(rand.NewPCG(uint64(index)+7, uint64(time.Now().UnixNano()))), size: tpl.EstimatedSize(), owner: owner,
	}, nil
}

// run paces records at rate until stepCtx ends; records are produced under the producer's run context so a step
// boundary never cancels buffered records.
func (l *lane) run(stepCtx context.Context, step int, rate float64) {
	pacer := pacing.NewPacer(rate, time.Now())
	ack := l.owner.onAck(step)
	idle := time.NewTimer(pacing.Limits.Idle)
	defer idle.Stop()
	for stepCtx.Err() == nil {
		due := pacer.Due(time.Now())
		var n uint64
		for ; n < uint64(due) && stepCtx.Err() == nil; n++ {
			l.owner.client.Produce(l.owner.runCtx, l.next(), ack)
		}
		l.owner.countSent(step, n)
		if due == 0 {
			idle.Reset(pacing.Limits.Idle)
			select {
			case <-stepCtx.Done():
			case <-idle.C:
			}
		}
	}
}

func (l *lane) next() *kgo.Record {
	i := l.picker.Pick(len(l.streams))
	seq := l.seqs[i]
	l.seqs[i]++
	name := l.streams[i]
	value := l.tpl.Append(make([]byte, 0, l.size), payload.Event{Seq: seq, Stream: name, Bad: l.bad > 0 && l.rng.Float64() < l.bad})
	return &kgo.Record{
		Topic: l.topics[i],
		Key:   l.picker.KeyFor(name),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: string(stream.HeaderSentNs), Value: strconv.AppendInt(nil, time.Now().UnixNano(), 10)},
			{Key: string(stream.HeaderSeq), Value: strconv.AppendInt(nil, seq, 10)},
			{Key: string(stream.HeaderStream), Value: []byte(name)},
		},
	}
}
