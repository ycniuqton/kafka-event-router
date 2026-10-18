package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth/workload"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/secrets"
)

type mapManager map[secrets.Ref][]byte

func (m mapManager) Put(context.Context, []byte) (secrets.Ref, error) { return "", nil }
func (m mapManager) Get(_ context.Context, r secrets.Ref) ([]byte, error) {
	v, ok := m[r]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return bytes.Clone(v), nil
}
func (m mapManager) Replace(context.Context, secrets.Ref, []byte) error { return errors.New("n/a") }
func (m mapManager) Delete(context.Context, secrets.Ref) error          { return nil }

type memRecorder struct{ entries []audit.Entry }

func (r *memRecorder) Record(_ context.Context, e audit.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func TestInternalSecretEndpoint(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	static, _ := workload.NewStaticTokenAuthenticator("tok")
	rec := &memRecorder{}
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}}, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: workload.NewManager(static), Responder: responder},
	})
	router := chi.NewRouter()
	m.Install(router)
	reader := secrets.NewAuditedReader(mapManager{"sec_a": []byte("s3cret")}, rec)
	m.Mount(router, NewModule(reader, responder))

	call := func(ref, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/internal/v1/secrets/"+ref, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	ok := call("sec_a", "tok")
	var body SecretResponse
	_ = json.Unmarshal(ok.Body.Bytes(), &body)
	plain, _ := base64.StdEncoding.DecodeString(body.ValueB64)
	if ok.Code != 200 || string(plain) != "s3cret" || ok.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("200 case: %d %s", ok.Code, ok.Body)
	}
	if len(rec.entries) != 1 || rec.entries[0].Actor.Kind != audit.ActorWorkload || rec.entries[0].RequestID == "" {
		t.Fatalf("audit: %+v", rec.entries)
	}
	if w := call("sec_a", "bad"); w.Code != 401 || bytes.Contains(w.Body.Bytes(), []byte("s3cret")) {
		t.Fatalf("bad token: %d", w.Code)
	}
	if w := call("sec_a", ""); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call("sec_missing", "tok"); w.Code != 404 {
		t.Fatalf("unknown ref: %d", w.Code)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("failed reads must not be audited as reads: %+v", rec.entries)
	}
}
