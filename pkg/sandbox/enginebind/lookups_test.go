package enginebind_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/host"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

func ptr[T any](v T) *T { return &v }

func TestScopeLookupsCollectsPinnedTables(t *testing.T) {
	cfg := &config.RouteConfig{Branches: []config.Branch{
		{Transform: &config.Transform{Steps: config.Steps{
			{Kind: config.StepKindLookup, Ref: ptr("countries"), Version: ptr(2)},
			{Kind: config.StepKindLookup, Rows: config.LookupRows{{Key: "a", Value: 1}}},
			{Kind: config.StepKindIfElse, ThenSteps: &config.Steps{{Kind: config.StepKindLookup, Ref: ptr("tiers"), Version: ptr(5)}},
				ElseSteps: &config.Steps{{Kind: config.StepKindLookup, Ref: ptr("currencies"), Version: ptr(1)}}},
		}}},
		{},
	}}
	got := enginebind.ScopeLookups{}.Of(cfg)
	want := []host.LookupPin{{Table: "countries", Version: 2}, {Table: "tiers", Version: 5}, {Table: "currencies", Version: 1}}
	if len(got) != len(want) {
		t.Fatalf("pins %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pin %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if (enginebind.ScopeLookups{}).Of(nil) != nil {
		t.Fatal("nil config has no pins")
	}
}

func libraryManager(t *testing.T, lib *configtest.InMemoryLibrary) sandbox.Manager {
	t.Helper()
	m, err := sandbox.NewManager(context.Background(), sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(),
		Salts:   sandboxtest.Salts{},
		Binder:  enginebind.Binder{Base: ctxhelpers.Set{Lookup: tables}},
		Library: enginebind.LibraryTables{Resolver: lib},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

const lookupCode = `function transform(e, ctx) {
  return { value: { tier: ctx.lookup('tiers', e.value.k), num: ctx.lookup('tiers', 7), base: ctx.lookup('countries', 'DE'), none: ctx.lookup('nope', 'x') } };
}`

// TestCtxLookupReadsPinnedLibraryVersion (JS-202): a route version reads the version it pinned, a
// newer publication does not change it, and inline/engine tables stay reachable.
func TestCtxLookupReadsPinnedLibraryVersion(t *testing.T) {
	lib := configtest.NewInMemoryLibrary()
	lib.PutLookupTable("tiers", 1, config.LookupRows{{Key: "gold", Value: "v1-gold"}, {Key: "7", Value: 7.0}})
	lib.PutLookupTable("tiers", 2, config.LookupRows{{Key: "gold", Value: "v2-gold"}})
	m := libraryManager(t, lib)
	ctx := context.Background()
	scope := sandboxtest.Scope(1)
	scope.LookupPins = []host.LookupPin{{Table: "tiers", Version: 1}}
	rs, err := m.Load(ctx, scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: lookupCode}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(ctx)
	lib.PutLookupTable("tiers", 1, config.LookupRows{{Key: "gold", Value: "mutated"}})
	mod, _ := rs.Module("/s")
	res, err := mod.Call(ctx, sandbox.Invocation{Event: boundary.GuestEvent{Value: map[string]any{"k": "gold"}}, Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Outputs[0].Value.(map[string]any)
	if got["tier"] != "v1-gold" || got["num"] != 7.0 || got["base"] != "Germany" || got["none"] != nil {
		t.Fatalf("lookup values %+v", got)
	}
}

func TestPinnedTablesFailTheLoad(t *testing.T) {
	lib := configtest.NewInMemoryLibrary()
	lib.PutLookupTable("tiers", 1, config.LookupRows{})
	cases := map[string][]host.LookupPin{
		"missing version": {{Table: "tiers", Version: 9}},
		"conflict":        {{Table: "tiers", Version: 1}, {Table: "tiers", Version: 2}},
	}
	m := libraryManager(t, lib)
	for name, pins := range cases {
		scope := sandboxtest.Scope(1)
		scope.LookupPins = pins
		_, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: lookupCode}}, sandbox.RouteLimits{})
		se, ok := sandbox.AsError(err)
		if !ok || se.Kind != sandbox.ErrorCompile || !strings.Contains(se.Message, "tiers") {
			t.Fatalf("%s: err %v, want a compile error naming the table", name, err)
		}
	}
	noLib := newManager(t)
	scope := sandboxtest.Scope(1)
	scope.LookupPins = []host.LookupPin{{Table: "tiers", Version: 1}}
	if _, err := noLib.Load(context.Background(), scope, nil, sandbox.RouteLimits{}); err == nil {
		t.Fatal("pins without a library port must fail")
	}
}
