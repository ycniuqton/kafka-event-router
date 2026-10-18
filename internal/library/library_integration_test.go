package library_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/library/importers"
	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
)

type libraryFixture struct {
	sb   *testkit.PostgresSandbox
	svc  *library.Service
	team uuid.UUID
}

func newLibraryFixture(t *testing.T) *libraryFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	checker, err := assist.NewStepChecker()
	if err != nil {
		t.Fatal(err)
	}
	compiler := library.EngineStepCompiler{Engine: compile.NewDefaultCompiler(),
		Options: library.StaticCompileOptions(engine.CompileOptions{Library: resolver.New(sb.DB)})}
	f := &libraryFixture{sb: sb, svc: library.NewService(library.ServiceDeps{
		Kinds: library.DefaultKindManager(checker, library.DefaultLimits, compiler), Repo: library.NewRepository(sb.DB),
		Usages: library.NewUsageRepository(sb.DB), Importers: importers.Register(library.DefaultLimits),
		Tx: store.NewTxManager(sb.DB), Limits: library.DefaultLimits})}
	if err := sb.DB.Pool().QueryRow(context.Background(), `INSERT INTO teams (name) VALUES ('payments') RETURNING id`).Scan(&f.team); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *libraryFixture) routeUsing(t *testing.T, step string) {
	t.Helper()
	doc := `{"branches":[{"id":"main","transform":{"steps":[` + step + `]}}]}`
	if _, err := f.sb.DB.Pool().Exec(context.Background(), `INSERT INTO routes (team_id, name, draft_config) VALUES ($1, 'r1', $2)`, f.team, doc); err != nil {
		t.Fatal(err)
	}
}

func problemKind(err error) problem.Kind {
	var c problem.Classified
	if errors.As(err, &c) {
		return c.ProblemKind()
	}
	return ""
}

// TestTransformLifecycle: create (v1 with hash and note), publish (v2), list with usage count, usages with pins,
// versions newest first, name conflict, archive refused while used, resolver loads pinned steps.
func TestTransformLifecycle(t *testing.T) {
	f := newLibraryFixture(t)
	ctx := context.Background()
	v1 := json.RawMessage(`[{"kind":"mask","field":"value.email","style":"email"},{"kind":"hash","field":"value.id"}]`)
	created, err := f.svc.Create(ctx, library.KindTransform, library.Draft{TeamID: f.team, Name: "mask-pii", Description: "PII", Content: v1, ChangeNote: "first"})
	if err != nil || created.LatestVersion != 1 || created.Size != 2 || created.TeamName != "payments" || created.Description != "PII" {
		t.Fatalf("create = %+v, %v", created, err)
	}
	published, err := f.svc.Publish(ctx, library.KindTransform, created.ID, library.VersionDraft{Content: json.RawMessage(`[{"kind":"hash","field":"value.id"}]`)})
	if err != nil || published.LatestVersion != 2 || published.Size != 1 {
		t.Fatalf("publish = %+v, %v", published, err)
	}
	f.routeUsing(t, `{"kind":"library","ref":"mask-pii","version":1}`)
	page, err := f.svc.List(ctx, library.KindTransform, library.ListQuery{Text: "MASK"})
	if err != nil || len(page.Items) != 1 || page.Items[0].UsageCount != 1 {
		t.Fatalf("list = %+v, %v", page, err)
	}
	usages, err := f.svc.Usages(ctx, library.KindTransform, created.ID)
	if err != nil || len(usages) != 1 || !usages[0].InDraft || len(usages[0].Versions) != 1 || usages[0].Versions[0] != 1 {
		t.Fatalf("usages = %+v, %v", usages, err)
	}
	versions, err := f.svc.Versions(ctx, library.KindTransform, created.ID)
	if err != nil || len(versions) != 2 || versions[0].No != 2 || versions[1].Size != 2 || versions[1].ChangeNote != "first" || versions[1].ConfigHash == "" {
		t.Fatalf("versions = %+v, %v", versions, err)
	}
	if _, err := f.svc.Create(ctx, library.KindTransform, library.Draft{TeamID: f.team, Name: "mask-pii", Content: v1}); problemKind(err) != problem.KindConflict {
		t.Fatalf("duplicate name err = %v", err)
	}
	var inUse library.InUseError
	if _, err := f.svc.Archive(ctx, library.KindTransform, created.ID); !errors.As(err, &inUse) || !strings.Contains(err.Error(), "r1") {
		t.Fatalf("archive in use err = %v", err)
	}
	steps, err := resolver.New(f.sb.DB).Transform(ctx, "mask-pii", 1)
	if err != nil || len(steps) != 2 {
		t.Fatalf("resolve = %v, %v", steps, err)
	}
	if _, err := resolver.New(f.sb.DB).Transform(ctx, "mask-pii", 9); !errors.Is(err, config.ErrLibraryRefMissing) {
		t.Fatalf("missing pin err = %v", err)
	}
}

// TestTransformMetadataArchiveAndCompile: If-Match metadata, archive of an unused transform, publish refused
// after archive, nested library refs compile only when the pin exists, cursor paging by name.
func TestTransformMetadataArchiveAndCompile(t *testing.T) {
	f := newLibraryFixture(t)
	ctx := context.Background()
	base, err := f.svc.Create(ctx, library.KindTransform, library.Draft{TeamID: f.team, Name: "base", Content: json.RawMessage(`[{"kind":"hash","field":"value.id"}]`)})
	if err != nil {
		t.Fatal(err)
	}
	desc := "hashed ids"
	if _, err := f.svc.UpdateMetadata(ctx, library.KindTransform, base.ID, library.MetadataPatch{Description: &desc}, base.RowVersion+7); problemKind(err) != problem.KindPreconditionFailed {
		t.Fatalf("stale If-Match err = %v", err)
	}
	updated, err := f.svc.UpdateMetadata(ctx, library.KindTransform, base.ID, library.MetadataPatch{Description: &desc}, base.RowVersion)
	if err != nil || updated.Description != desc || updated.RowVersion == base.RowVersion {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	nested := library.Draft{TeamID: f.team, Name: "wrapper", Content: json.RawMessage(`[{"kind":"library","ref":"base","version":5}]`)}
	var verr *api.ValidationError
	if _, err := f.svc.Create(ctx, library.KindTransform, nested); !errors.As(err, &verr) || !strings.HasPrefix(verr.Errors[0].Path, "/steps/0") {
		t.Fatalf("missing nested pin err = %v", err)
	}
	nested.Content = json.RawMessage(`[{"kind":"library","ref":"base","version":1}]`)
	if _, err := f.svc.Create(ctx, library.KindTransform, nested); err != nil {
		t.Fatalf("nested create: %v", err)
	}
	first, err := f.svc.List(ctx, library.KindTransform, library.ListQuery{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Items[0].Name != "base" || first.Next == nil {
		t.Fatalf("page 1 = %+v, %v", first, err)
	}
	second, err := f.svc.List(ctx, library.KindTransform, library.ListQuery{Limit: 1, After: first.Next})
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "wrapper" || second.Next != nil {
		t.Fatalf("page 2 = %+v, %v", second, err)
	}
	if _, err := f.svc.Archive(ctx, library.KindTransform, second.Items[0].ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := f.svc.Publish(ctx, library.KindTransform, second.Items[0].ID, library.VersionDraft{Content: nested.Content}); problemKind(err) != problem.KindConflict {
		t.Fatalf("publish archived err = %v", err)
	}
	if all, _ := f.svc.List(ctx, library.KindTransform, library.ListQuery{Names: []string{"base", "wrapper"}}); len(all.Items) != 1 {
		t.Fatalf("archived listed: %+v", all.Items)
	}
}

// TestLookupTableImportAndRows: typed keys, CSV import with per-line issues (nothing saved), paged rows, resolver.
func TestLookupTableImportAndRows(t *testing.T) {
	f := newLibraryFixture(t)
	ctx := context.Background()
	_, err := f.svc.Create(ctx, library.KindLookupTable, library.Draft{TeamID: f.team, Name: "regions", Content: json.RawMessage(`[{"key":"","value":1}]`)})
	var verr *api.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("invalid rows err = %v", err)
	}
	it, err := f.svc.Create(ctx, library.KindLookupTable, library.Draft{TeamID: f.team, Name: "regions", KeyType: library.ValueString,
		Content: json.RawMessage(`[{"key":"VN","value":"APAC"}]`)})
	if err != nil || it.Size != 1 || it.KeyType != library.ValueString {
		t.Fatalf("create = %+v, %v", it, err)
	}
	bad := "key,value\nDE,EMEA\nDE,EU\n,x\n"
	if _, err := f.svc.Import(ctx, it.ID, string(library.ImportCSV), strings.NewReader(bad), library.VersionDraft{}); !errors.As(err, &verr) || len(verr.Errors) != 2 || verr.Errors[0].Path != "/lines/3" {
		t.Fatalf("bad csv err = %v", err)
	}
	good := "\ufeffkey,value\nDE,EMEA\nUS,AMER\n\"FR\",\"EMEA, west\"\n"
	updated, err := f.svc.Import(ctx, it.ID, "text/csv; charset=utf-8", strings.NewReader(good), library.VersionDraft{ChangeNote: "csv"})
	if err != nil || updated.LatestVersion != 2 || updated.Size != 3 {
		t.Fatalf("import = %+v, %v", updated, err)
	}
	page, err := f.svc.Rows(ctx, it.ID, library.RowsQuery{Version: 2, Limit: 2})
	if err != nil || len(page.Rows) != 2 || page.Total != 3 || page.Next == nil || page.Rows[1].Key != "US" {
		t.Fatalf("rows page = %+v, %v", page, err)
	}
	rest, err := f.svc.Rows(ctx, it.ID, library.RowsQuery{Version: 2, Offset: 2, Limit: 2})
	if err != nil || len(rest.Rows) != 1 || rest.Next != nil || string(rest.Rows[0].Value) != `"EMEA, west"` {
		t.Fatalf("rows rest = %+v, %v", rest, err)
	}
	rows, err := resolver.New(f.sb.DB).LookupTable(ctx, "regions", 2)
	if err != nil || len(rows) != 3 || rows[0].Key != "DE" {
		t.Fatalf("resolve rows = %+v, %v", rows, err)
	}
	team, err := f.svc.Owner(library.KindLookupTable).TeamOf(ctx, it.ID.String())
	if err != nil || team != f.team.String() {
		t.Fatalf("TeamOf = %q, %v", team, err)
	}
	if _, err := f.svc.Versions(ctx, library.KindLookupTable, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing versions err = %v", err)
	}
}
