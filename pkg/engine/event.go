package engine

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// Header is one Kafka record header.
type Header struct {
	Key   string
	Value []byte
}

// RawRecord is a record as bytes, before decode or after encode.
type RawRecord struct {
	Key       []byte
	Value     []byte
	Headers   []Header
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Event is a decoded record flowing through a pipeline.
type Event struct {
	Key       any
	Value     any
	Headers   []Header
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// BranchID names the route branch an output belongs to.
type BranchID string

// Out is an output event tagged with the branch that produced it.
type Out struct {
	Event
	Branch BranchID
}

// RouteMeta identifies the running route version (metadata value sources, hash salt scope).
type RouteMeta struct {
	RouteID string
	Name    string
	Version int
	TeamID  string
}

// Pipeline is a compiled route version. Process may mutate the input event; callers
// that need the original must Clone it first.
type Pipeline interface {
	Decode(ctx context.Context, r RawRecord) (Event, error)
	Process(ctx context.Context, in Event, tr trace.Tracer) ([]Out, error)
	Encode(ctx context.Context, o Out) (RawRecord, error)
	Close(ctx context.Context) error
}

// Compiler turns a validated route document into a Pipeline (once per version);
// compile failures are returned as *CompileError.
type Compiler interface {
	Compile(ctx context.Context, doc *config.Document, opts CompileOptions) (Pipeline, error)
}

// CompileError lists every compile issue with its JSON Pointer.
type CompileError struct {
	Issues config.Issues
}

func (e *CompileError) Error() string {
	if len(e.Issues) == 0 {
		return "compile failed"
	}
	return "compile failed: " + string(e.Issues[0].Path) + ": " + e.Issues[0].Message
}
