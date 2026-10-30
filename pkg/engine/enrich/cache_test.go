package enrich_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/enrich"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestAnswerCacheTTLAndEviction(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	c := enrich.NewAnswerCache(enrich.CacheSettings{TTL: time.Minute, NegativeTTL: time.Second, MaxEntries: 2}, clock.now)
	c.Put("a", ctxhelpers.SourceAnswer{Found: true, Value: map[string]any{"x": int64(1)}})
	c.Put("miss", ctxhelpers.SourceAnswer{})
	got, ok := c.Get("a")
	if !ok || got.Value.(map[string]any)["x"] != int64(1) {
		t.Fatalf("a: %+v %v", got, ok)
	}
	got.Value.(map[string]any)["x"] = int64(99)
	if again, _ := c.Get("a"); again.Value.(map[string]any)["x"] != int64(1) {
		t.Fatal("cached values must be copied out")
	}
	clock.t = clock.t.Add(2 * time.Second)
	if _, ok := c.Get("miss"); ok {
		t.Fatal("negative entry must expire after NegativeTTL")
	}
	c.Put("b", ctxhelpers.SourceAnswer{Found: true})
	c.Put("c", ctxhelpers.SourceAnswer{Found: true})
	if _, ok := c.Get("a"); ok || c.Len() != 2 {
		t.Fatalf("least recently used entry must be evicted, len=%d", c.Len())
	}
	clock.t = clock.t.Add(2 * time.Minute)
	if _, ok := c.Get("b"); ok {
		t.Fatal("found entry must expire after TTL")
	}
}

func TestAnswerCacheSkipsDisabledKinds(t *testing.T) {
	c := enrich.NewAnswerCache(enrich.CacheSettings{TTL: time.Minute, MaxEntries: 10}, nil)
	c.Put("miss", ctxhelpers.SourceAnswer{})
	if c.Len() != 0 {
		t.Fatal("misses are not cached without negativeTtlMs")
	}
	if _, on := (enrich.CacheSettingsOf{}).Of(&config.LookupCache{}); on {
		t.Fatal("an empty cache member enables nothing")
	}
}

func TestLookupCompileIssues(t *testing.T) {
	cases := map[string]string{
		`{"kind":"sql","sql":"DELETE FROM t"}`:                  "/branches/0/transform/steps/0/lookup/request/sql",
		`{"kind":"sql","sql":"SELECT * FROM t WHERE id = :id"}`: "/branches/0/transform/steps/0/lookup/request/params/id",
		`{"kind":"redis","op":"mget"}`:                          "/branches/0/transform/steps/0/lookup/request/keys",
		`{"kind":"redis","op":"hget","key":"k"}`:                "/branches/0/transform/steps/0/lookup/request/hashField",
		`{"kind":"http","path":"/x/${nope.a}"}`:                 "/branches/0/transform/steps/0/lookup/request/path",
		`{"kind":"static"}`:                                     "/branches/0/transform/steps/0/lookup/request/key",
	}
	for req, want := range cases {
		steps := `[{"kind":"lookup","lookup":{"sourceId":"s","request":` + req + `,"map":[{"from":"$","to":"value.x"}]}}]`
		doc, _ := config.Parse([]byte(enginetest.RouteJSON("null", steps)))
		_, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, enginetest.GoldenOptions())
		var ce *engine.CompileError
		if !errors.As(err, &ce) || !strings.Contains(issuePaths(ce), want) {
			t.Fatalf("%s: want issue at %s, got %v", req, want, err)
		}
	}
}

func TestLookupCompileRejectsBadPick(t *testing.T) {
	steps := `[{"kind":"lookup","lookup":{"sourceId":"s","request":{"kind":"static","key":"k"},"map":[{"from":"tier","to":"value.x"}]}}]`
	doc, _ := config.Parse([]byte(enginetest.RouteJSON("null", steps)))
	_, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, enginetest.GoldenOptions())
	var ce *engine.CompileError
	if !errors.As(err, &ce) || !strings.Contains(issuePaths(ce), "/lookup/map/0/from") {
		t.Fatalf("got %v", err)
	}
}

func issuePaths(ce *engine.CompileError) string {
	var b strings.Builder
	for _, i := range ce.Issues {
		b.WriteString(string(i.Path) + " ")
	}
	return b.String()
}
