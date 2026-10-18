package enginetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
)

// RouteJSON builds a single-branch JSON route around a filter (JSON or "null") and a steps array.
func RouteJSON(filter, steps string) string {
	return fmt.Sprintf(`{"apiVersion":"router/v1","name":"test","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","filter":%s,"transform":{"steps":%s},"destination":{"connectionId":"c","topic":"out"}}]}`, filter, steps)
}

// MustCompile compiles raw with the golden options or fails the test.
func MustCompile(tb testing.TB, raw string) engine.Pipeline {
	tb.Helper()
	return MustCompileWith(tb, raw, GoldenOptions())
}

// MustCompileWith compiles raw with opts or fails the test; the pipeline closes with the test.
func MustCompileWith(tb testing.TB, raw string, opts engine.CompileOptions) engine.Pipeline {
	tb.Helper()
	doc, err := config.Parse([]byte(raw))
	if err != nil {
		tb.Fatal(err)
	}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, opts)
	if err != nil {
		tb.Fatalf("compile: %v", err)
	}
	tb.Cleanup(func() { _ = p.Close(context.Background()) })
	return p
}

// MustDecode decodes a JSON value line into an event.
func MustDecode(tb testing.TB, p engine.Pipeline, value string) engine.Event {
	tb.Helper()
	ev, err := p.Decode(context.Background(), engine.RawRecord{Value: []byte(value), Topic: "in", Timestamp: GoldenNow})
	if err != nil {
		tb.Fatal(err)
	}
	return ev
}
