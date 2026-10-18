package preview_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
)

// TestRunJSLookupReadsPinnedLibraryTable checks preview/worker parity: ctx.lookup in a JS step reads the
// library table the route pins with a named lookup step.
func TestRunJSLookupReadsPinnedLibraryTable(t *testing.T) {
	lib := configtest.NewInMemoryLibrary()
	lib.PutLookupTable("countries", 1, config.LookupRows{{Key: "DE", Value: "Germany"}})
	salts := preview.NewProcessSaltProvider()
	scripts, err := preview.NewLibrarySandboxScripts(context.Background(), sandbox.DefaultConfig(), salts, enginebind.LibraryTables{Resolver: lib})
	if err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	defer scripts.Close(context.Background())
	b, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), scripts, salts)
	if err != nil {
		t.Fatal(err)
	}
	b.Library = lib
	r := preview.PreviewRunner{Builder: b, Differ: preview.StructuralDiffer{}, Deadline: preview.RunDeadline.Duration(), Now: time.Now}
	route := `{"apiVersion":"router/v1","name":"js-lookup","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[
 {"id":"l","kind":"lookup","field":"value.c","to":"value.name","ref":"countries","version":1},
 {"id":"j","kind":"js","code":"function transform(event, ctx) { return { value: Object.assign({}, event.value, { js: ctx.lookup('countries', event.value.c) }) }; }"}]},
"destination":{"connectionId":"c","topic":"out"}}]}`
	recs := []preview.SampleRecord{{Seq: 0, Value: []byte(`{"c":"DE"}`), Timestamp: goldenStart}}
	res, err := r.Run(context.Background(), preview.RunPlan{Config: []byte(route), Records: recs, Loaded: 1, Scope: preview.RunScope{TeamID: "t", Mode: preview.RunPreview}})
	if err != nil {
		t.Fatal(err)
	}
	out := mustJSON(res.Results[0].Branches[0].Outputs)
	if res.Results[0].Status != preview.StatusPassed || !strings.Contains(out, `"js":"Germany"`) || !strings.Contains(out, `"name":"Germany"`) {
		t.Fatalf("result %+v outputs %s", res.Results[0], out)
	}
}
