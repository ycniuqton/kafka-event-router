package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	audithttp "github.com/eventrouter/router/internal/audit/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

// OpenAPIContract and the media types come from rbactest (shared with the approvals contract test).
type OpenAPIContract = rbactest.OpenAPIContract

// MediaType aliases the contract media type.
type MediaType = rbactest.MediaType

const (
	MediaJSON    = rbactest.MediaJSON
	MediaProblem = rbactest.MediaProblem
)

// NewOpenAPIContract loads the spec.
func NewOpenAPIContract(t *testing.T) *OpenAPIContract {
	return rbactest.NewOpenAPIContract(t)
}

// TestUsersAndRoleBindingsMatchOpenAPI (AC-40) drives the real handlers and validates every body against the spec.
func TestUsersAndRoleBindingsMatchOpenAPI(t *testing.T) {
	f, contract := newFixture(t), NewOpenAPIContract(t)
	const spec = "access-control.yaml"
	user := "/api/v1/users/" + f.editor.String()
	binding := "/api/v1/role-bindings/" + f.editor.String() + "/" + team.String()
	f.w.Users[f.admin].WorkspaceAdmin = true
	steps := []struct {
		as                       bool
		method, path, body, etag string
		key                      string
		status                   int
		media                    MediaType
	}{
		{true, http.MethodGet, "/api/v1/users", "", "", "users", http.StatusOK, MediaJSON},
		{true, http.MethodGet, user, "", "", "user", http.StatusOK, MediaJSON},
		{true, http.MethodPatch, user, `{"displayName":"Hoa"}`, `"1"`, "user", http.StatusOK, MediaJSON},
		{false, http.MethodPatch, user, `{"displayName":"x"}`, `"2"`, "user", http.StatusForbidden, MediaProblem},
		{true, http.MethodGet, "/api/v1/role-bindings", "", "", "roleBindings", http.StatusOK, MediaJSON},
		{true, http.MethodPut, binding, `{"role":"admin"}`, `"1"`, "roleBinding", http.StatusOK, MediaJSON},
		{true, http.MethodPut, binding, `{"role":"editor"}`, `"9"`, "roleBinding", http.StatusPreconditionFailed, MediaProblem},
		{true, http.MethodDelete, "/api/v1/role-bindings/" + f.admin.String() + "/" + team.String(), "", `"1"`, "roleBinding", http.StatusNoContent, ""},
	}
	for _, s := range steps {
		as := f.editor
		if s.as {
			as = f.admin
		}
		rec, _ := f.do(as, s.method, s.path, s.body, s.etag)
		if rec.Code != s.status {
			t.Fatalf("%s %s: status %d, want %d: %s", s.method, s.path, rec.Code, s.status, rec.Body)
		}
		if s.media != "" {
			contract.Check(t, spec, s.key, s.method, rec.Code, s.media, rec.Body.Bytes())
		}
	}
}

// fixedEntries is an audit EntryReader returning one page of canned entries.
type fixedEntries []audit.LogEntry

func (f fixedEntries) Query(context.Context, audit.Filter, audit.Page) ([]audit.LogEntry, *store.Cursor, error) {
	return f, &store.Cursor{SortKey: []string{"k"}, ID: "1"}, nil
}

// TestAuditLogMatchesOpenAPI validates GET /audit-log and /audit-log/actions bodies against the spec.
func TestAuditLogMatchesOpenAPI(t *testing.T) {
	contract, actor, teamID := NewOpenAPIContract(t), uuid.New(), team
	entries := fixedEntries{
		{ID: 2, At: time.Unix(20, 0).UTC(), Actor: audit.LogActor{ID: &actor, Name: "Hoa", Kind: audit.ActorUser}, Action: audit.ActionRouteCreated,
			ObjectType: audit.ObjectRoute, ObjectID: "r1", ObjectLabel: "orders", TeamID: &teamID, IP: "10.0.0.1", RequestID: "req"},
		{ID: 1, At: time.Unix(10, 0).UTC(), Actor: audit.LogActor{Kind: audit.ActorSystem}, Action: audit.ActionDevSeed,
			ObjectType: audit.ObjectUser, ObjectID: "u1", Diff: []byte(`{"after":{"role":"admin"}}`)},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mod := audithttp.NewModule(audithttp.Deps{Audit: audit.NewManager(entries, audit.NewCatalog()),
		Responder: api.NewResponder(problem.NewCatalog(), log), Pages: api.NewPageParser(store.NewCursorCodec("test"))})
	for i, key := range []string{"auditLog", "auditActions"} {
		h, rec := mod.Handlers()[i], httptest.NewRecorder()
		if err := h.Serve(rec, httptest.NewRequest(http.MethodGet, h.Route().Pattern, nil)); err != nil {
			t.Fatal(err)
		}
		contract.Check(t, "access-control.yaml", key, http.MethodGet, http.StatusOK, MediaJSON, rec.Body.Bytes())
	}
}

// TestTeamsGrantsAndInvitationsMatchOpenAPI (AC-134) drives the P2 handlers and validates bodies against the spec.
func TestTeamsGrantsAndInvitationsMatchOpenAPI(t *testing.T) {
	f, contract := newFixture(t), NewOpenAPIContract(t)
	f.w.Users[f.admin].WorkspaceAdmin = true
	const spec = "access-control.yaml"
	teamPath := "/api/v1/teams/" + team.String()
	grant := teamPath + "/grants/" + otherConn.String()
	steps := []struct {
		as                       bool
		method, path, body, etag string
		key                      string
		status                   int
		media                    MediaType
	}{
		{false, http.MethodGet, "/api/v1/teams", "", "", "teams", http.StatusOK, MediaJSON},
		{true, http.MethodPost, "/api/v1/teams", `{"name":"Payments"}`, "", "teams", http.StatusCreated, MediaJSON},
		{false, http.MethodPost, "/api/v1/teams", `{"name":"Risk"}`, "", "teams", http.StatusForbidden, MediaProblem},
		{true, http.MethodPost, "/api/v1/teams", `{"name":"payments"}`, "", "teams", http.StatusConflict, MediaProblem},
		{true, http.MethodGet, teamPath, "", "", "team", http.StatusOK, MediaJSON},
		{true, http.MethodPatch, teamPath, `{"requireProdApproval":false}`, `"1"`, "team", http.StatusOK, MediaJSON},
		{true, http.MethodPatch, teamPath, `{"name":""}`, `"2"`, "team", http.StatusUnprocessableEntity, MediaProblem},
		{true, http.MethodPut, grant, "", "", "teamGrant", http.StatusOK, MediaJSON},
		{false, http.MethodGet, teamPath + "/grants", "", "", "teamGrants", http.StatusOK, MediaJSON},
		{true, http.MethodDelete, grant, "", "", "teamGrant", http.StatusNoContent, ""},
		{true, http.MethodDelete, grant, "", "", "teamGrant", http.StatusNotFound, MediaProblem},
		{true, http.MethodPost, "/api/v1/users/invitations", `{"email":"Ana@Example.io","teamId":"` + team.String() + `","role":"viewer"}`, "", "invitations", http.StatusCreated, MediaJSON},
		{true, http.MethodPost, "/api/v1/users/invitations", `{"email":"ana@example.io","teamId":"` + team.String() + `","role":"viewer"}`, "", "invitations", http.StatusConflict, MediaProblem},
		{true, http.MethodPost, "/api/v1/users/invitations", `{"email":"nope","teamId":"` + team.String() + `","role":"viewer"}`, "", "invitations", http.StatusUnprocessableEntity, MediaProblem},
	}
	for _, s := range steps {
		as := f.editor
		if s.as {
			as = f.admin
		}
		rec, _ := f.do(as, s.method, s.path, s.body, s.etag)
		if rec.Code != s.status {
			t.Fatalf("%s %s: status %d, want %d: %s", s.method, s.path, rec.Code, s.status, rec.Body)
		}
		if s.media != "" {
			contract.Check(t, spec, s.key, s.method, rec.Code, s.media, rec.Body.Bytes())
		}
	}
}

// TestWorkspaceSettingsMatchOpenAPI (AC-206) drives GET/PATCH /workspace-settings through the real middleware.
func TestWorkspaceSettingsMatchOpenAPI(t *testing.T) {
	f, contract := newFixture(t), NewOpenAPIContract(t)
	f.w.Users[f.admin].WorkspaceAdmin = true
	const path, key = "/api/v1/workspace-settings", "workspaceSettings"
	steps := []struct {
		as                 bool
		method, body, etag string
		status             int
		media              MediaType
	}{
		{false, http.MethodGet, "", "", http.StatusOK, MediaJSON},
		{false, http.MethodPatch, `{"aiAssistEnabled":true}`, `"1"`, http.StatusForbidden, MediaProblem},
		{true, http.MethodPatch, `{"aiAssistEnabled":true}`, "", http.StatusPreconditionRequired, MediaProblem},
		{true, http.MethodPatch, `{"aiAssistEnabled":true}`, `"1"`, http.StatusOK, MediaJSON},
		{true, http.MethodPatch, `{"aiAssistEnabled":false}`, `"1"`, http.StatusPreconditionFailed, MediaProblem},
		{false, http.MethodGet, "", "", http.StatusOK, MediaJSON},
	}
	for _, s := range steps {
		as := f.editor
		if s.as {
			as = f.admin
		}
		rec, body := f.do(as, s.method, path, s.body, s.etag)
		if rec.Code != s.status {
			t.Fatalf("%s %s: status %d, want %d: %s", s.method, s.etag, rec.Code, s.status, rec.Body)
		}
		contract.Check(t, "access-control.yaml", key, s.method, rec.Code, s.media, rec.Body.Bytes())
		if s.method == http.MethodGet && s.etag == "" && rec.Header().Get("ETag") == "" {
			t.Fatalf("GET without ETag: %v", body)
		}
	}
	if last := f.w.Entries[len(f.w.Entries)-1]; last.Action != audit.ActionSettingsUpdated {
		t.Fatalf("settings change not audited: %+v", last)
	}
}

// exportStatus is a fixed ExportStatusReader.
type exportStatus struct{ s audit.ExportStatus }

func (e exportStatus) Status(context.Context) (audit.ExportStatus, error) { return e.s, nil }

// TestAuditExportMatchesOpenAPI (AC-202/204): the status body and the unknown-format problem match the spec,
// a CSV export streams with a download name and records audit.exported.
func TestAuditExportMatchesOpenAPI(t *testing.T) {
	contract, w := NewOpenAPIContract(t), rbactest.NewWorld()
	now := time.Date(2026, 10, 14, 9, 5, 0, 0, time.UTC)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	cp := audit.Checkpoint{Sink: audit.ExportSinkWebhook, LastID: 41, ExportedTotal: 41, LastError: "503", LastAttemptAt: &now, UpdatedAt: &now}
	mod := audithttp.NewExportModule(audithttp.Deps{Audit: audit.NewManager(fixedEntries{{ID: 1, At: now, Action: audit.ActionRouteCreated}}, audit.NewCatalog()),
		Responder: responder, Recorder: w, Now: func() time.Time { return now },
		Export: exportStatus{audit.ExportStatus{Enabled: true, Sink: audit.ExportSinkWebhook, Target: "https://siem.example/in", Checkpoint: &cp, LagRows: 3}}})
	export, status := mod.Handlers()[0], mod.Handlers()[1]
	rec := httptest.NewRecorder()
	if err := status.Serve(rec, httptest.NewRequest(http.MethodGet, string(audithttp.PathAuditExportStatus), nil)); err != nil {
		t.Fatal(err)
	}
	contract.Check(t, "access-control.yaml", "auditExportStatus", http.MethodGet, http.StatusOK, MediaJSON, rec.Body.Bytes())
	rec = httptest.NewRecorder()
	responder.Problem(rec, httptest.NewRequest(http.MethodGet, "/x", nil),
		export.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, string(audithttp.PathAuditExport)+"?format=xlsx", nil)))
	contract.Check(t, "access-control.yaml", "auditExport", http.MethodGet, http.StatusUnprocessableEntity, MediaProblem, rec.Body.Bytes())
	rec = httptest.NewRecorder()
	if err := export.Serve(rec, httptest.NewRequest(http.MethodGet, string(audithttp.PathAuditExport)+"?format=csv&sensitive=true", nil)); err != nil {
		t.Fatal(err)
	}
	if rec.Header().Get("Content-Disposition") != `attachment; filename="audit-log-20261014-090500.csv"` || !strings.HasPrefix(rec.Body.String(), "id,at,") {
		t.Fatalf("download: %v %s", rec.Header(), rec.Body)
	}
	if len(w.Entries) != 1 || w.Entries[0].Action != audit.ActionAuditExported || !strings.Contains(string(w.Entries[0].Diff), "sensitive=true") {
		t.Fatalf("export not audited: %+v", w.Entries)
	}
}
