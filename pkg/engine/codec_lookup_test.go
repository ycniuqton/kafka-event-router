package engine_test

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// EchoMarker steers the echo gateway: requests containing it miss or fail.
type EchoMarker string

const (
	echoMiss EchoMarker = "miss"
	echoDown EchoMarker = "down"
)

// echoSources answers every lookup with its own rendered request, so fast and tree only agree
// when both codecs render identical requests from the event.
func echoSources() *enginetest.FakeSourceGateway {
	return enginetest.NewFakeSourceGateway(func(call ctxhelpers.SourceCall) enginetest.FakeAnswer {
		raw, _ := json.Marshal(call.Request)
		text := string(raw)
		if strings.Contains(text, string(echoDown)) {
			return enginetest.FakeAnswer{Err: &ctxhelpers.SourceError{Transient: true, Reason: "source unavailable"}}
		}
		if strings.Contains(text, string(echoMiss)) {
			return enginetest.FakeAnswer{}
		}
		return enginetest.FakeAnswer{Found: true, Value: map[string]any{"echo": text, "kind": string(call.Kind), "rows": []any{text, len(text)}}}
	})
}

// lookupParityRoutes cover every request kind, template position, miss/error policy and a later step reading the answer.
var lookupParityRoutes = map[string]string{
	"http": enginetest.RouteJSON(`{"type":"cond","field":"value.id","op":"exists"}`, `[
{"kind":"lookup","id":"crm","lookup":{"sourceId":"s1","request":{"kind":"http","method":"POST","path":"/c/${value.id}",
 "query":{"t":"${headers.h}","r":"${value.a.b}"},"headers":{"x":"${value.x}"},"body":{"id":"${value.id}","items":["${value.items[0].sku}",1]}},
 "map":[{"from":"$.echo","to":"value.enriched.echo"},{"from":"$.kind","to":"value.a.kind"}],"onMiss":{"action":"default","value":"none"},"onError":{"action":"skip"}}},
{"kind":"rename","from":"value.enriched.echo","to":"value.e2"}]`),
	"redis_sql_static": enginetest.RouteJSON("null", `[
{"kind":"lookup","id":"r1","lookup":{"sourceId":"s2","request":{"kind":"redis","op":"get","key":"k:${value.id}"},"map":[{"from":"$.echo","to":"value.r.get"}],"onMiss":{"action":"skip"}}},
{"kind":"lookup","id":"r2","lookup":{"sourceId":"s2","request":{"kind":"redis","op":"mget","keys":["a:${value.x}","b:${value.a.b}"]},"map":[{"from":"$.rows[1]","to":"value.r.n"}],"onError":{"action":"default","value":0}}},
{"kind":"lookup","id":"q1","lookup":{"sourceId":"s3","request":{"kind":"sql","sql":"SELECT 1 WHERE :id = :id AND :b = :b","params":{"id":"${value.id}","b":"${value.b}"}},
 "map":[{"from":"$","to":"value.row"}],"onMiss":{"action":"drop"},"cache":{"ttlMs":60000,"maxEntries":16}}},
{"kind":"lookup","id":"st","lookup":{"sourceId":"s4","request":{"kind":"static","key":"${value.cc}"},"map":[{"from":"$.kind","to":"value.static.kind"}],"onMiss":{"action":"fail"}}}]`),
}

// lookupParityEvents hit, miss (marker), fail (marker) and leave template fields missing.
var lookupParityEvents = []string{
	`{"id":"i-1","a":{"b":1},"x":"X","b":"low","cc":"DE","items":[{"sku":"S1"}],"keep":{"deep":[1,2]}}`,
	`{"id":"miss","a":{"b":"miss"},"x":"down","b":2,"cc":"miss"}`,
	`{"id":"down","x":"X","cc":"FR"}`,
	`{"id":7,"a":null,"items":[],"cc":["array"]}`,
	`{"z":1}`,
}

func TestFastCodecLookupRouteParity(t *testing.T) {
	keys := []string{"id", "a", "b", "x", "cc", "items", "sku", "keep", "enriched", "r", "row", "static"}
	for name, route := range lookupParityRoutes {
		t.Run(name, func(t *testing.T) {
			opts := enginetest.GoldenOptions()
			opts.Helpers.Sources = echoSources()
			pair := newCodecPair(t, []byte(route), opts)
			if !pair.fast.CodecPlan().Fast() {
				t.Fatalf("plan %s, want fast", pair.fast.CodecPlan())
			}
			mut := enginetest.EventMutator{R: rand.New(rand.NewPCG(23, uint64(len(name)))), Keys: keys}
			for i, ev := range lookupParityEvents {
				pair.check(t, []byte(ev), int64(i))
				for v := 0; v < int(parityRouteSeeds); v++ {
					pair.check(t, mut.Variant([]byte(ev)), int64(v))
				}
			}
		})
	}
}
