package engine_test

import (
	"math/rand/v2"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// parityRoutes exercise every path-bounded step kind on nested, partial, indexed and
// missing paths, multiple branches and destination key fields.
var parityRoutes = map[string]string{
	"fields": enginetest.RouteJSON(`{"type":"and","children":[{"type":"cond","field":"value.a.b","op":"exists"},{"type":"not","child":{"type":"cond","field":"value.items[0].sku","op":"eq","value":"X"}}]}`,
		`[{"kind":"rename","from":"value.a.b","to":"value.a.c.d"},{"kind":"copy","from":"value.x","to":"value.a.copy"},{"kind":"move","from":"value.id","to":"key"},
{"kind":"remove","fields":["value.a.gone","value.items[1]","value.b"]},{"kind":"set","field":"value.n.m.o","source":{"from":"field","path":"value.items[0]"}},
{"kind":"default_if_missing","field":"value.a.def","value":{"z":[1,"2"]}}]`),
	"values": enginetest.RouteJSON("null",
		`[{"kind":"mask","field":"value.email","style":"email"},{"kind":"hash","field":"value.a.x"},{"kind":"convert_type","field":"value.amount","targetType":"string","onFailure":"keep"},
{"kind":"round","field":"value.price","decimals":1},{"kind":"string_case","field":"value.b","case":"upper","to":"value.B"},
{"kind":"redact_pattern","field":"value.text","preset":"email"},{"kind":"format_date","field":"value.at","layout":"date","timezone":"UTC"},
{"kind":"flatten","field":"value.a"},{"kind":"set","field":"value.f","source":{"from":"formula","expr":"concat(value.x, \"-\", value.a.b)"}},
{"kind":"set","field":"value.meta.topic","source":{"from":"metadata","name":"topic"}},{"kind":"set_key","source":{"from":"field","path":"value.id"}}]`),
	"flow": enginetest.RouteJSON(`{"type":"or","children":[{"type":"cond","field":"value.x","op":"exists"},{"type":"cond","field":"value.a","op":"is_empty"}]}`,
		`[{"kind":"if_else","condition":{"type":"cond","field":"value.a.b","op":"gt","value":0},"thenSteps":[{"kind":"set","field":"value.a.big","source":{"from":"constant","value":true}}],
"elseSteps":[{"kind":"remove","fields":["value.a.b"]}]},{"kind":"drop_if","condition":{"type":"cond","field":"value.drop","op":"eq","value":true}},
{"kind":"lookup","field":"value.cc","to":"value.country","rows":[{"key":"DE","value":"Germany"},{"key":"1","value":{"n":1}}],"defaultValue":null},
{"kind":"split_array","field":"value.items","to":"value.item"}]`),
	"headers": enginetest.RouteJSON("null",
		`[{"kind":"header_add","header":"h2","source":{"from":"field","path":"value.a"}},{"kind":"header_copy_to_payload","header":"h","field":"value.hdr.h"},
{"kind":"header_copy_from_payload","field":"value.x","header":"x"},{"kind":"header_rename","header":"h2","newHeader":"h3"},{"kind":"header_remove","header":"h"}]`),
	"branches": `{"apiVersion":"router/v1","name":"p","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},"branches":[
 {"id":"a","transform":{"steps":[{"kind":"set","field":"value.a.x","source":{"from":"constant","value":"a"}}]},"destination":{"connectionId":"c","topic":"ta","key":{"mode":"field","field":"value.id"}}},
 {"id":"b","filter":{"type":"cond","field":"value.a.x","op":"not_exists"},"transform":{"steps":[{"kind":"remove","fields":["value.a.b"]}]},"destination":{"connectionId":"c","topic":"tb","headers":{"mode":"drop_all"}}},
 {"id":"c","filter":{"type":"cond","field":"value.a.b","op":"exists"},"destination":{"connectionId":"c","topic":"tc","key":{"mode":"none"}}}]}`,
}

// parityEvents are the base events mutated for the synthetic routes.
var parityEvents = []string{
	`{"a":{"b":1,"gone":2,"x":"ax"},"x":"X","id":"i-1","items":[{"sku":"S1","qty":1},{"sku":"S2"}],"email":"jo@x.io","amount":12.5,"price":1.25,"b":"low","text":"mail jo@x.io now","at":"2026-01-02T03:04:05Z","cc":"DE","hdr":{"k":1}}`,
	`{"a":null,"x":[1,2],"items":[],"drop":true}`,
	`{"a":{"b":-1,"c":{"d":0}},"items":"not an array","cc":1,"at":1767225600000}`,
	`{"z":{"deep":{"er":[{"a":{"b":1}}]}},"a":{}}`,
	`{}`,
}

// parityKeys steer mutations towards the members the synthetic routes touch.
var parityKeys = []string{"a", "b", "c", "d", "x", "id", "items", "sku", "email", "amount", "price", "text", "at", "cc", "drop", "hdr", "gone", "n", "m"}

func TestFastCodecRouteParity(t *testing.T) {
	for name, route := range parityRoutes {
		t.Run(name, func(t *testing.T) {
			pair := newCodecPair(t, []byte(route), enginetest.GoldenOptions())
			if !pair.fast.CodecPlan().Fast() {
				t.Fatalf("plan %s, want fast", pair.fast.CodecPlan())
			}
			mut := enginetest.EventMutator{R: rand.New(rand.NewPCG(11, uint64(len(name)))), Keys: parityKeys}
			for i, ev := range parityEvents {
				pair.check(t, []byte(ev), int64(i))
				for v := 0; v < int(parityRouteSeeds); v++ {
					pair.check(t, mut.Variant([]byte(ev)), int64(v))
				}
			}
			t.Logf("fallbacks: %d of %d", pair.fast.(engine.CodecStatsReporter).CodecStats().Fallbacks, len(parityEvents)*(int(parityRouteSeeds)+1))
		})
	}
}

// parityRouteNames orders the synthetic routes for the fuzz selector.
var parityRouteNames = []string{"fields", "values", "flow", "headers", "branches"}

func FuzzFastVsTree(f *testing.F) {
	for _, ev := range parityEvents {
		f.Add([]byte(ev), uint8(0))
	}
	pairs := make([]codecPair, 0, len(parityRouteNames))
	for _, n := range parityRouteNames {
		pairs = append(pairs, newCodecPair(f, []byte(parityRoutes[n]), enginetest.GoldenOptions()))
	}
	f.Fuzz(func(t *testing.T, raw []byte, route uint8) {
		pairs[int(route)%len(pairs)].check(t, raw, 0)
	})
}
