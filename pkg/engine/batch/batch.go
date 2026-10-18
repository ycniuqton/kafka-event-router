// Package batch runs a compiled pipeline over a JSONL stream of events (routerctl,
// golden tests): outputs and typed errors are written as JSONL, traces optionally.
package batch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// LineLimit bounds one input line.
type LineLimit int

const MaxLineBytes LineLimit = 16 << 20

// OutputRecord is one produced record on the outputs stream.
type OutputRecord struct {
	Branch  string             `json:"branch"`
	Topic   string             `json:"topic"`
	Key     any                `json:"key"`
	Value   any                `json:"value"`
	Headers []engine.HeaderDoc `json:"headers"`
}

// ErrorRecord is one failed event on the errors stream.
type ErrorRecord struct {
	Offset  int64  `json:"offset"`
	Type    string `json:"type"`
	Step    string `json:"step,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Sinks are the output streams; a nil Traces disables tracing.
type Sinks struct {
	Outputs io.Writer
	Errors  io.Writer
	Traces  io.Writer
}

// Summary counts what a run did.
type Summary struct {
	Events  int `json:"events"`
	Outputs int `json:"outputs"`
	Errors  int `json:"errors"`
}

// LineEncoder turns one input line into record value bytes (e.g. JSON → Avro wire format).
type LineEncoder interface {
	Encode(ctx context.Context, line []byte) ([]byte, error)
}

// ValueRenderer turns produced value bytes back into a printable value (e.g. Avro → map).
type ValueRenderer interface {
	Decode(ctx context.Context, raw []byte) (any, error)
}

// Runner feeds every non-empty line as the record value (offset = event index); optional
// Input/Output adapt binary wire formats to and from JSON lines.
type Runner struct {
	Pipeline  engine.Pipeline
	Topic     string
	Timestamp time.Time
	Input     LineEncoder
	Output    ValueRenderer
}

// Run processes the whole stream; only I/O failures are returned as errors.
func (r Runner) Run(ctx context.Context, in io.Reader, sinks Sinks) (Summary, error) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), int(MaxLineBytes))
	w := writers{out: json.NewEncoder(sinks.Outputs), errs: json.NewEncoder(sinks.Errors)}
	if sinks.Traces != nil {
		w.traces = json.NewEncoder(sinks.Traces)
	}
	sum := Summary{}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		value, err := r.value(ctx, line)
		if err != nil {
			return sum, err
		}
		rec := engine.RawRecord{Value: value, Topic: r.Topic, Offset: int64(sum.Events), Timestamp: r.Timestamp}
		if err := r.one(ctx, rec, w, &sum); err != nil {
			return sum, err
		}
		sum.Events++
	}
	return sum, sc.Err()
}

// value returns the record bytes of an input line.
func (r Runner) value(ctx context.Context, line []byte) ([]byte, error) {
	if r.Input == nil {
		return append([]byte(nil), line...), nil
	}
	return r.Input.Encode(ctx, line)
}

// one runs decode → process → encode for a record and writes the results.
func (r Runner) one(ctx context.Context, rec engine.RawRecord, w writers, sum *Summary) error {
	tr := w.tracer(rec.Offset)
	outs, err := r.process(ctx, rec, tr)
	if err == nil {
		err = r.emit(ctx, outs, w, sum)
	}
	if err != nil {
		var pe *engine.ProcessError
		if !errors.As(err, &pe) {
			return err
		}
		sum.Errors++
		if werr := w.errs.Encode(ErrorRecords{}.Of(rec.Offset, pe)); werr != nil {
			return werr
		}
	}
	return w.trace(tr)
}

func (r Runner) process(ctx context.Context, rec engine.RawRecord, tr trace.Tracer) ([]engine.Out, error) {
	ev, err := r.Pipeline.Decode(ctx, rec)
	if err != nil {
		tr.Finish(trace.StatusError, &trace.ErrorInfo{Type: string(engine.ErrDecode), Message: err.Error()})
		return nil, err
	}
	return r.Pipeline.Process(ctx, ev, tr)
}

// emit encodes all outputs first so a failing output leaves none of the event's outputs written.
func (r Runner) emit(ctx context.Context, outs []engine.Out, w writers, sum *Summary) error {
	records := make([]OutputRecord, 0, len(outs))
	for _, o := range outs {
		raw, err := r.Pipeline.Encode(ctx, o)
		if err != nil {
			return err
		}
		rec := OutputRecords{}.Of(o.Branch, raw)
		if r.Output != nil && raw.Value != nil {
			if rec.Value, err = r.Output.Decode(ctx, raw.Value); err != nil {
				return err
			}
		}
		records = append(records, rec)
	}
	for _, rec := range records {
		if err := w.out.Encode(rec); err != nil {
			return err
		}
		sum.Outputs++
	}
	return nil
}

// writers are the JSONL encoders of one run.
type writers struct {
	out, errs, traces *json.Encoder
}

func (w writers) tracer(offset int64) trace.Tracer {
	if w.traces == nil {
		return trace.NoopTracer{}
	}
	return trace.NewRecordingTracer(offset)
}

func (w writers) trace(tr trace.Tracer) error {
	rt, ok := tr.(*trace.RecordingTracer)
	if !ok {
		return nil
	}
	return w.traces.Encode(rt.Trace())
}

// OutputRecords renders encoded records for JSONL output.
type OutputRecords struct{}

// Of renders key/value bytes as JSON when valid JSON, else as text.
func (OutputRecords) Of(branch engine.BranchID, raw engine.RawRecord) OutputRecord {
	rec := OutputRecord{Branch: string(branch), Topic: raw.Topic, Key: OutputRecords{}.bytes(raw.Key), Value: OutputRecords{}.bytes(raw.Value), Headers: []engine.HeaderDoc{}}
	for _, h := range raw.Headers {
		rec.Headers = append(rec.Headers, engine.HeaderDoc{Key: h.Key, Value: string(h.Value)})
	}
	return rec
}

func (OutputRecords) bytes(b []byte) any {
	if b == nil {
		return nil
	}
	if json.Valid(b) {
		return json.RawMessage(b)
	}
	return string(b)
}

// ErrorRecords renders processing errors.
type ErrorRecords struct{}

// Of renders pe for the event at offset.
func (ErrorRecords) Of(offset int64, pe *engine.ProcessError) ErrorRecord {
	msg := pe.Error()
	if pe.Cause != nil {
		msg = pe.Cause.Error()
	}
	return ErrorRecord{Offset: offset, Type: string(pe.Type), Step: pe.StepID, Path: string(pe.Path), Message: msg}
}
