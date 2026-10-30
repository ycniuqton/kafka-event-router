package config_test

import (
	"fmt"
	"testing"
)

func lookupRoute(step string) string {
	return fmt.Sprintf(`{"apiVersion":"router/v1","name":"t","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[%s]},"destination":{"connectionId":"c","topic":"out"}}]}`, step)
}

func TestSchemaLookupSourceMode(t *testing.T) {
	valid := []string{
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"http","method":"POST","path":"/c/${value.id}","query":{"a":"b"},"body":{"id":"${value.id}","n":1}},
		  "map":[{"from":"$.tier","to":"value.tier"}],"timeoutMs":200,"retries":2,"onMiss":{"action":"default","value":{"x":1}},
		  "onError":{"action":"skip"},"cache":{"ttlMs":1000,"negativeTtlMs":100,"maxEntries":50}}}`,
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"redis","op":"mget","keys":["a:${value.id}"]},"map":[{"from":"$[0]","to":"value.a"}]}}`,
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"sql","sql":"SELECT 1 WHERE :id = :id","params":{"id":"${value.id}"}},"map":[{"from":"$","to":"value.row"}]}}`,
		`{"kind":"lookup","field":"value.country","rows":[{"key":"DE","value":"EU"}]}`,
	}
	for _, s := range valid {
		if issues := schemaIssues(t, lookupRoute(s)); len(issues) != 0 {
			t.Fatalf("%s: unexpected issues %+v", s, issues)
		}
	}
	invalid := []string{
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"http"},"map":[{"from":"$","to":"value.x"}]}}`,
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"static","key":"k"},"map":[]}}`,
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"static","key":"k"},"map":[{"from":"tier","to":"value.x"}]}}`,
		`{"kind":"lookup","lookup":{"sourceId":"s1","request":{"kind":"static","key":"k"},"map":[{"from":"$","to":"value.x"}],"onMiss":{"action":"explode"}}}`,
		`{"kind":"lookup","rows":[{"key":"DE","value":"EU"}]}`,
	}
	for _, s := range invalid {
		if issues := schemaIssues(t, lookupRoute(s)); len(issues) == 0 {
			t.Fatalf("%s: want schema issues", s)
		}
	}
}
