// Package stream is the contract between loadgen and the stress runner: Kafka headers stamped on every event and the
// JSON-lines report loadgen writes to stdout.
package stream

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/seqtrack"
)

// Header names stamped by the producer and read by the measurer.
type Header string

const (
	HeaderSentNs Header = "stress-sent-ns"
	HeaderSeq    Header = "stress-seq"
	HeaderStream Header = "stress-stream"
)

// Kind tags one JSON line of a loadgen report.
type Kind string

const (
	KindReady   Kind = "ready"
	KindStep    Kind = "step"
	KindWindow  Kind = "window"
	KindSummary Kind = "summary"
	KindError   Kind = "error"
)

// Role says which loadgen subcommand wrote a line.
type Role string

const (
	RoleProduce Role = "produce"
	RoleMeasure Role = "measure"
)

// StepReport is one load step as achieved by the producer.
type StepReport struct {
	Index        int     `json:"index"`
	TargetRate   float64 `json:"targetRate"`
	StartNs      int64   `json:"startNs"`
	EndNs        int64   `json:"endNs,omitempty"`
	DurationSec  float64 `json:"durationSec"`
	Sent         uint64  `json:"sent"`
	Acked        uint64  `json:"acked"`
	Errors       uint64  `json:"errors"`
	AchievedRate float64 `json:"achievedRate"`
}

// ProduceWindow is the producer's progress over one report interval.
type ProduceWindow struct {
	Step      int     `json:"step"`
	SentRate  float64 `json:"sentRate"`
	AckedRate float64 `json:"ackedRate"`
	Errors    uint64  `json:"errors"`
	Buffered  int64   `json:"buffered"`
}

// MeasureWindow is what the measurer saw over one report interval.
type MeasureWindow struct {
	Count     uint64            `json:"count"`
	Rate      float64           `json:"rate"`
	Latency   latency.Summary   `json:"latency"`
	Histogram latency.Sparse    `json:"histogram"`
	ByTopic   map[string]uint64 `json:"byTopic,omitempty"`
}

// ProduceSummary is the final producer report.
type ProduceSummary struct {
	Steps        []StepReport `json:"steps"`
	Sent         uint64       `json:"sent"`
	Acked        uint64       `json:"acked"`
	Errors       uint64       `json:"errors"`
	DurationSec  float64      `json:"durationSec"`
	AchievedRate float64      `json:"achievedRate"`
	FirstError   string       `json:"firstError,omitempty"`
}

// MeasureSummary is the final measurer report.
type MeasureSummary struct {
	Latency     latency.Summary   `json:"latency"`
	Delivery    seqtrack.Summary  `json:"delivery"`
	ByTopic     map[string]uint64 `json:"byTopic"`
	DurationSec float64           `json:"durationSec"`
	Unparsed    uint64            `json:"unparsed"`
}

// Line is one JSON line; exactly one payload member is set, matching Kind.
type Line struct {
	Kind    Kind            `json:"kind"`
	Role    Role            `json:"role"`
	AtNs    int64           `json:"atNs"`
	Step    *StepReport     `json:"step,omitempty"`
	Produce *ProduceWindow  `json:"produce,omitempty"`
	Measure *MeasureWindow  `json:"measure,omitempty"`
	PSum    *ProduceSummary `json:"produceSummary,omitempty"`
	MSum    *MeasureSummary `json:"measureSummary,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Writer serialises report lines to one output (stdout), safe for concurrent use.
type Writer struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewWriter writes JSON lines to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{enc: json.NewEncoder(w)}
}

// Emit writes one line; write errors are ignored because the report is best effort.
func (w *Writer) Emit(l Line) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(l)
}
