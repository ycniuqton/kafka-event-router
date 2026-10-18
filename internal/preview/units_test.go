package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
	"github.com/eventrouter/router/pkg/kafka"
)

type patchCase struct {
	Name   string          `json:"name"`
	Before any             `json:"before"`
	After  any             `json:"after"`
	Patch  []trace.PatchOp `json:"patch"`
}

// TestPatchFixtures checks Diff and Apply against the shared Go↔TS fixtures (testdata/patch).
func TestPatchFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/patch/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []patchCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := preview.StructuralDiffer{}.Diff(tc.Before, tc.After)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.Patch)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("diff = %s, want %s", gotJSON, wantJSON)
			}
			applied, err := preview.PatchApplier{}.Apply(tc.Before, tc.Patch)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(applied, tc.After) {
				t.Errorf("apply = %v, want %v", applied, tc.After)
			}
		})
	}
}

func TestPatchDifferNumbersAcrossTypes(t *testing.T) {
	ops := preview.StructuralDiffer{}.Diff(map[string]any{"n": int64(5)}, map[string]any{"n": 5.0})
	if len(ops) != 0 {
		t.Fatalf("int64 5 vs float 5 should be equal, got %+v", ops)
	}
}

func newBudget() *kafka.SamplingBudget {
	return kafka.NewSamplingBudget(int(preview.MaxEvents), int64(preview.MaxBytes), time.Time{})
}

func uploadSource() preview.UploadSource {
	return preview.UploadSource{Clock: func() time.Time { return time.Unix(1000, 0) }}
}

func TestUploadSource(t *testing.T) {
	cases := []struct {
		name    string
		req     preview.LoadRequest
		budget  *kafka.SamplingBudget
		loaded  int
		reason  kafka.StopReason
		errPath string
	}{
		{name: "json array of values", req: preview.LoadRequest{Upload: `[{"a":1},{"a":2}]`}, loaded: 2, reason: kafka.StopComplete},
		{name: "ndjson event shape", req: preview.LoadRequest{Upload: "{\"key\":\"k1\",\"value\":{\"a\":1}}\n\n{\"value\":2}\n"}, loaded: 2, reason: kafka.StopComplete},
		{name: "bad ndjson line", req: preview.LoadRequest{Upload: "{\"a\":1}\n{oops\n"}, errPath: "/upload"},
		{name: "empty", req: preview.LoadRequest{Upload: "   "}, errPath: "/events"},
		{name: "structured events", req: preview.LoadRequest{Events: []preview.InlineEvent{{Value: json.RawMessage(`{"x":1}`)}}}, loaded: 1, reason: kafka.StopComplete},
		{name: "structured event without value", req: preview.LoadRequest{Events: []preview.InlineEvent{{Key: json.RawMessage(`"k"`)}}}, errPath: "/events/0"},
		{name: "truncated by count", req: preview.LoadRequest{Upload: `[1,2,3]`}, budget: kafka.NewSamplingBudget(2, 100, time.Time{}), loaded: 2, reason: kafka.StopMaxEvents},
		{name: "truncated by bytes", req: preview.LoadRequest{Upload: `["aaaa","bbbb"]`}, budget: kafka.NewSamplingBudget(10, 7, time.Time{}), loaded: 1, reason: kafka.StopMaxBytes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.budget
			if b == nil {
				b = newBudget()
			}
			res, err := uploadSource().Load(context.Background(), tc.req, b)
			if tc.errPath != "" {
				var pe *problem.Error
				if !errors.As(err, &pe) || pe.Path != tc.errPath {
					t.Fatalf("err = %v, want problem at %s", err, tc.errPath)
				}
				return
			}
			if err != nil || len(res.Records) != tc.loaded || res.StopReason != tc.reason {
				t.Fatalf("res = %d records %s, err %v", len(res.Records), res.StopReason, err)
			}
		})
	}
}

func TestUploadKeepsKeysHeadersAndTopic(t *testing.T) {
	ts := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	req := preview.LoadRequest{Topics: []string{"orders"}, Events: []preview.InlineEvent{{
		Key: json.RawMessage(`"o-1"`), Value: json.RawMessage(`{"a":1}`), Headers: []engine.HeaderDoc{{Key: "h", Value: "v"}}, Timestamp: &ts,
	}}}
	res, err := uploadSource().Load(context.Background(), req, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Records[0]
	if string(r.Key) != "o-1" || r.Topic != "orders" || r.Partition != -1 || !r.Timestamp.Equal(ts) || string(r.Headers[0].Value) != "v" {
		t.Fatalf("record = %+v", r)
	}
}

func TestSampleSearcher(t *testing.T) {
	recs := []preview.SampleRecord{
		{Seq: 0, Key: []byte("Order-1"), Value: []byte(`{"t":"OrderPaid"}`)},
		{Seq: 1, Value: []byte(`{"t":"Created"}`), Headers: []engine.Header{{Key: "traceId", Value: []byte("ABC")}}},
		{Seq: 2, Value: []byte(`{"t":"x"}`)},
	}
	cases := map[string][]int{"": {0, 1, 2}, "orderpaid": {0}, "abc": {1}, "TRACEID": {1}, "order-1": {0}, "nope": {}}
	for q, want := range cases {
		got := []int{}
		for _, r := range (preview.SampleSearcher{}).Filter(recs, q) {
			got = append(got, r.Seq)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("q=%q got %v want %v", q, got, want)
		}
	}
}

func findNode(n preview.FieldNode, path string) (preview.FieldNode, bool) {
	if n.Path == path {
		return n, true
	}
	for _, c := range n.Children {
		if f, ok := findNode(c, path); ok {
			return f, true
		}
	}
	return preview.FieldNode{}, false
}

func TestSchemaInferrer(t *testing.T) {
	docs := []any{}
	for i := 0; i < 60; i++ {
		v := map[string]any{"eventType": []string{"OrderPaid", "OrderCreated"}[i%2], "id": float64(i), "items": []any{map[string]any{"sku": "A"}}}
		if i%5 == 0 {
			v["amount"] = "100"
		} else {
			v["amount"] = float64(i)
		}
		docs = append(docs, map[string]any{"key": nil, "value": v, "headers": map[string]any{}})
	}
	fs := preview.NewSchemaInferrer().Infer(docs)
	et, ok := findNode(fs.Root, "value.eventType")
	if !ok || et.Frequency != 1 || len(et.TopValues) != 2 || et.TopValues[0].Pct != 0.5 {
		t.Fatalf("eventType = %+v", et)
	}
	amount, _ := findNode(fs.Root, "value.amount")
	if amount.Types[preview.TypeString] != 12 || amount.Types[preview.TypeNumber] != 48 {
		t.Errorf("amount types = %v", amount.Types)
	}
	id, _ := findNode(fs.Root, "value.id")
	if len(id.TopValues) != 0 {
		t.Errorf("high-cardinality id kept top values: %d", len(id.TopValues))
	}
	if _, ok := findNode(fs.Root, "value.items[].sku"); !ok {
		t.Error("array element path missing")
	}
}

type storeFixture struct {
	store preview.SessionStore
	now   *time.Time
	ff    func(time.Duration)
}

func storeFixtures(t *testing.T) map[string]storeFixture {
	now := time.Unix(1000, 0)
	mem := storeFixture{now: &now}
	mem.store = preview.NewMemorySessionStore(func() time.Time { return *mem.now })
	mem.ff = func(d time.Duration) { *mem.now = mem.now.Add(d) }
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	rs := storeFixture{store: preview.NewRedisSessionStore(client), now: &now, ff: mr.FastForward}
	return map[string]storeFixture{"memory": mem, "redis": rs}
}

func TestSessionStores(t *testing.T) {
	for name, fx := range storeFixtures(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			m := preview.NewSessionStoreManager(fx.store, func() time.Time { return *fx.now })
			owner := preview.Caller{UserID: "u1"}
			recs := []preview.SampleRecord{{Seq: 0, Value: []byte("a")}, {Seq: 1, Value: []byte("b"), Headers: []engine.Header{{Key: "h", Value: []byte{0xff}}}}}
			s, err := m.Create(ctx, preview.Session{ID: "ps_1", OwnerID: "u1", Topics: []string{"t"}}, recs)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Owned(ctx, s.ID, preview.Caller{UserID: "other"}); !errors.Is(err, preview.ErrSessionNotFound) {
				t.Fatalf("foreign session err = %v", err)
			}
			page, err := m.Records(ctx, s.ID, 1, 5)
			if err != nil || len(page) != 1 || page[0].Headers[0].Value[0] != 0xff {
				t.Fatalf("page = %+v err %v", page, err)
			}
			fx.ff(50 * time.Minute)
			if _, err := m.Touch(ctx, s); err != nil {
				t.Fatal(err)
			}
			fx.ff(50 * time.Minute)
			if _, err := m.Owned(ctx, s.ID, owner); err != nil {
				t.Fatalf("touched session expired early: %v", err)
			}
			if err := m.PutSchema(ctx, s.ID, preview.FieldSchema{Samples: 2}); err != nil {
				t.Fatal(err)
			}
			if fs, ok, _ := m.Schema(ctx, s.ID); !ok || fs.Samples != 2 {
				t.Fatalf("schema = %+v %v", fs, ok)
			}
			fx.ff(61 * time.Minute)
			if _, err := m.Owned(ctx, s.ID, owner); !errors.Is(err, preview.ErrSessionExpired) {
				t.Fatalf("expired err = %v", err)
			}
		})
	}
}

func TestDirectiveCodec(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	codec := preview.NewDirectiveCodec("secret", clock)
	v, err := codec.Encode(preview.Caller{UserID: "u1", TeamID: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := codec.Decode(v); err != nil || c.UserID != "u1" {
		t.Fatalf("decode = %+v %v", c, err)
	}
	if _, err := preview.NewDirectiveCodec("other", clock).Decode(v); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, err := codec.Decode(strings.Replace(v, ".", "x.", 1)); err == nil {
		t.Fatal("tampered payload accepted")
	}
	now = now.Add(2 * time.Minute)
	if _, err := codec.Decode(v); err == nil {
		t.Fatal("expired directive accepted")
	}
}
