package record

import (
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine"
)

func sample() Record {
	return Record{
		Key: []byte("k1"), Value: []byte(`{"a":1}`),
		Headers: []engine.Header{{Key: "traceId", Value: []byte("ab")}, {Key: "router.step", Value: []byte("old")}},
		Failure: Failure{RouteID: "r1", VersionNo: 4, Branch: "main", Step: "s1", ErrorType: ErrorStep,
			Message: "boom", Stack: "at x", FailedAt: time.Date(2026, 10, 13, 9, 41, 7, 0, time.UTC)},
		Source: SourcePosition{Topic: "orders", Partition: 2, Offset: 884162},
	}
}

func TestCodecRoundTrip(t *testing.T) {
	raw := Codec{}.Encode(sample())
	raw.Topic, raw.Partition, raw.Offset = "router.dlq.x", 1, 7
	got, err := Codec{}.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := sample()
	if got.Failure != want.Failure || got.Source != want.Source {
		t.Fatalf("got %+v / %+v", got.Failure, got.Source)
	}
	if got.Ref != (Ref{Topic: "router.dlq.x", Partition: 1, Offset: 7}) {
		t.Fatalf("ref %+v", got.Ref)
	}
	if len(got.Headers) != 1 || got.Headers[0].Key != "traceId" {
		t.Fatalf("headers %+v", got.Headers)
	}
}

func TestCodecTruncates(t *testing.T) {
	r := sample()
	r.Failure.Message = strings.Repeat("é", int(MaxMessageBytes))
	r.Failure.Stack = strings.Repeat("x", int(MaxStackBytes)+10)
	got, err := Codec{}.Decode(Codec{}.Encode(r))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Failure.Message) > int(MaxMessageBytes) || !strings.HasPrefix(r.Failure.Message, got.Failure.Message) {
		t.Fatalf("message len %d", len(got.Failure.Message))
	}
	if len(got.Failure.Stack) != int(MaxStackBytes) {
		t.Fatalf("stack len %d", len(got.Failure.Stack))
	}
}

func TestCodecUnreadable(t *testing.T) {
	cases := map[string][]engine.Header{
		"missing":       nil,
		"bad type":      {{Key: string(HeaderRouteID), Value: []byte("r")}, {Key: string(HeaderErrorType), Value: []byte("nope")}},
		"bad offset":    {{Key: string(HeaderRouteID), Value: []byte("r")}, {Key: string(HeaderErrorType), Value: []byte("timeout")}, {Key: string(HeaderSourceOffset), Value: []byte("x")}},
		"bad timestamp": {{Key: string(HeaderRouteID), Value: []byte("r")}, {Key: string(HeaderErrorType), Value: []byte("timeout")}, {Key: string(HeaderFailedAt), Value: []byte("x")}},
	}
	ts := time.Unix(100, 0)
	for name, hs := range cases {
		got, err := Codec{}.Decode(engine.RawRecord{Headers: hs, Timestamp: ts})
		if err != ErrUnreadable || got.Failure.ErrorType != ErrorUnknown || got.Failure.Message != string(MessageUnreadable) {
			t.Fatalf("%s: %v %+v", name, err, got.Failure)
		}
	}
}

func TestTopicNaming(t *testing.T) {
	if got := (TopicNaming{}).Default("Order totals → analytics"); got != "router.dlq.order-totals---analytics" {
		t.Fatal(got)
	}
}

func TestFromEngine(t *testing.T) {
	if FromEngine(engine.ErrStepTimeout) != ErrorTimeout || FromEngine("other") != ErrorStep {
		t.Fatal("mapping")
	}
}

func FuzzCodecDecode(f *testing.F) {
	f.Add("router.version", "12", []byte("v"))
	f.Add("router.failed_at", "bad", []byte{0xff})
	f.Fuzz(func(t *testing.T, key, value string, body []byte) {
		_, _ = Codec{}.Decode(engine.RawRecord{Value: body, Headers: []engine.Header{{Key: key, Value: []byte(value)}}})
	})
}
