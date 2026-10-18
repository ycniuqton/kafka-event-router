package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/kafka"
)

var demoNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func loadDemo(t *testing.T, req preview.LoadRequest) preview.LoadResult {
	t.Helper()
	res, err := preview.NewDemoSource(func() time.Time { return demoNow }).Load(context.Background(), req, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDemoSourceIsDeterministicOrderEvents(t *testing.T) {
	req := preview.LoadRequest{Mode: preview.ModeDemo, Count: 30}
	a, b := loadDemo(t, req), loadDemo(t, req)
	if len(a.Records) != 30 || !reflect.DeepEqual(a, b) || a.StopReason != kafka.StopComplete {
		t.Fatalf("records %d stop %s equal %v", len(a.Records), a.StopReason, reflect.DeepEqual(a, b))
	}
	last := a.Records[29]
	if last.Topic != string(kafka.DemoOrderEvents) || !last.Timestamp.Equal(demoNow) || a.Records[0].Timestamp.After(last.Timestamp) {
		t.Fatalf("last record = %+v", last)
	}
	var ev kafka.DemoOrderEvent
	if err := json.Unmarshal(last.Value, &ev); err != nil || ev.OrderID != string(last.Key) || ev.EventType == "" || len(last.Headers) != 2 {
		t.Fatalf("event %+v err %v headers %v", ev, err, last.Headers)
	}
}

func TestDemoSourceDefaultsAndLimits(t *testing.T) {
	if got := loadDemo(t, preview.LoadRequest{Mode: preview.ModeDemo}); len(got.Records) != int(preview.DefaultCount) {
		t.Fatalf("default count = %d", len(got.Records))
	}
	if got := loadDemo(t, preview.LoadRequest{Mode: preview.ModeDemo, Count: 5000}); len(got.Records) != int(preview.MaxEvents) {
		t.Fatalf("clamped count = %d", len(got.Records))
	}
	tight := kafka.NewSamplingBudget(int(preview.MaxEvents), 2000, demoNow.Add(time.Minute))
	res, err := preview.NewDemoSource(func() time.Time { return demoNow }).Load(context.Background(), preview.LoadRequest{Mode: preview.ModeDemo, Count: 100}, tight)
	if err != nil || res.StopReason != kafka.StopMaxBytes || len(res.Records) == 0 || res.Bytes > 2000 {
		t.Fatalf("byte cap: %d records %d bytes %s %v", len(res.Records), res.Bytes, res.StopReason, err)
	}
}

func TestDemoSourceRejectsOtherTopics(t *testing.T) {
	_, err := preview.NewDemoSource(time.Now).Load(context.Background(), preview.LoadRequest{Mode: preview.ModeDemo, Topics: []string{"order-events", "payments"}}, newBudget())
	var pe *problem.Error
	if !errors.As(err, &pe) || pe.Path != "/topics/1" || pe.Kind != preview.KindTopicNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestSampleValueMaskerMasksEverySample(t *testing.T) {
	m := preview.NewSampleValueMasker()
	out := m.Mask(preview.MaskTypePreserving, []json.RawMessage{json.RawMessage(`{"a":"secret","n":7}`), json.RawMessage(`not json`)})
	if string(out[0]) != `{"a":"••••••","n":0}` || string(out[1]) != `"••••••"` {
		t.Fatalf("masked = %s / %s", out[0], out[1])
	}
}
