package connhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/pkg/kafka"
)

// teamRoleAuthz mirrors the role-binding engine: reads are open across teams, writes need editor+ on the
// object's team (any team for collection targets).
type teamRoleAuthz struct {
	seen []auth.ObjectRef
}

func (a *teamRoleAuthz) Authorize(_ context.Context, p auth.Principal, perm auth.Permission, obj auth.ObjectRef) error {
	a.seen = append(a.seen, obj)
	if perm.Access == auth.AccessRead {
		return nil
	}
	for _, tr := range p.Teams {
		writer := tr.Role == auth.RoleEditor || tr.Role == auth.RoleAdmin
		if writer && (obj.TeamID == "" || obj.TeamID == tr.TeamID.String()) {
			return nil
		}
	}
	return auth.ErrForbidden
}

// foreignConnection stores a connection owned by another team directly in the repository.
func (h *harness) foreignConnection(t *testing.T, team uuid.UUID) connections.Connection {
	t.Helper()
	ref, err := h.vault.Store(t.Context(), kafka.SecretBundle{Password: secretPassword})
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.repo.Create(t.Context(), connections.Connection{
		TeamID: team, Name: "analytics", EnvTag: connections.EnvDev, BootstrapServers: []string{"127.0.0.1:1"}, Kind: kafka.ClusterKafka,
		Security: connections.Security{Mode: kafka.SecuritySASLPlain, Auth: kafka.AuthSpec{Username: "svc"}}, SecretRef: ref,
		SecretFields: []kafka.SecretField{kafka.SecretPassword}, Health: connections.Health{Status: connections.HealthUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTeamScopedWritesOnConnections(t *testing.T) {
	viewerTeam := uuid.New()
	authz := &teamRoleAuthz{}
	h := newHarnessWith(t, harnessOptions{authz: authz, extraTeams: []auth.TeamRole{{TeamID: viewerTeam, TeamName: "Analytics", Role: auth.RoleViewer}}})
	foreign := h.foreignConnection(t, viewerTeam)
	item := "/api/v1/connections/" + foreign.ID.String()

	resp, raw := h.do(t, http.MethodGet, item, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read across teams must be open: %d %s", resp.StatusCode, raw)
	}
	if got := authz.seen[len(authz.seen)-1]; got.TeamID != viewerTeam.String() || got.ID != foreign.ID.String() {
		t.Fatalf("object must carry the owner team: %+v", got)
	}
	resp, _ = h.do(t, http.MethodPatch, item, `{"description":"x"}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer edit: %d", resp.StatusCode)
	}
	for _, path := range []string{item + "/test", item + "/topics"} {
		if resp, _ = h.do(t, http.MethodPost, path, `{"name":"t"}`, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("viewer POST %s: %d", path, resp.StatusCode)
		}
	}
	if resp, _ = h.do(t, http.MethodDelete, item, "", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer delete: %d", resp.StatusCode)
	}
	draft := `{"name":"x","envTag":"dev","bootstrapServers":["127.0.0.1:1"],"security":{"mode":"sasl_plain","username":"svc"},"connectionId":"` + foreign.ID.String() + `"}`
	if resp, raw = h.do(t, http.MethodPost, "/api/v1/connections/test", draft, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("draft test reusing a foreign connection's secrets: %d %s", resp.StatusCode, raw)
	}
	body := `{"name":"mine","envTag":"dev","owner":{"teamId":"` + viewerTeam.String() + `"},"bootstrapServers":["127.0.0.1:1"],"security":{"mode":"plaintext"}}`
	if resp, raw = h.do(t, http.MethodPost, "/api/v1/connections", body, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("create in a viewer team: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/connections", `{"name":"mine","envTag":"dev","bootstrapServers":["127.0.0.1:1"],"security":{"mode":"plaintext"}}`, nil)
	var created connhttp.ConnectionDTO
	_ = json.Unmarshal(raw, &created)
	if resp.StatusCode != http.StatusCreated || created.Owner.TeamID != h.team {
		t.Fatalf("create in own editor team: %d %s", resp.StatusCode, raw)
	}
	resp, _ = h.do(t, http.MethodPatch, "/api/v1/connections/"+created.ID.String(), `{"sensitive":true}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("editor edit own team: %d", resp.StatusCode)
	}
}

func TestOAuthAndSchemaRegistryResourceShape(t *testing.T) {
	h := newHarness(t)
	body := `{"name":"oauth","envTag":"dev","bootstrapServers":["127.0.0.1:1"],
	  "security":{"mode":"sasl_oauthbearer","clientId":"router","tokenUrl":"https://idp/token","scopes":["kafka"]},
	  "schemaRegistry":{"url":"https://sr:8081","auth":{"mode":"basic","username":"sr"}},
	  "secrets":{"clientSecret":"` + secretPassword + `","schemaRegistrySecret":"` + secretPassword + `x"}}`
	resp, raw := h.do(t, http.MethodPost, "/api/v1/connections", body, nil)
	var c connhttp.ConnectionDTO
	_ = json.Unmarshal(raw, &c)
	if resp.StatusCode != http.StatusCreated || c.Security.ClientID != "router" || c.Security.SecretFields[kafka.SecretClientSecret] != connhttp.SecretSet {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}
	if c.SchemaRegistry == nil || c.SchemaRegistry.Secret == nil || *c.SchemaRegistry.Secret != connhttp.SecretSet {
		t.Fatalf("registry block: %s", raw)
	}
	item := "/api/v1/connections/" + c.ID.String()
	resp, raw = h.do(t, http.MethodPatch, item, `{"schemaRegistry":{"auth":{"mode":"bearer","username":null}}}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switch to bearer keeps the stored secret: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPatch, item, `{"schemaRegistry":null}`, map[string]string{"If-Match": `"2"`})
	_ = json.Unmarshal(raw, &c)
	if resp.StatusCode != http.StatusOK || c.SchemaRegistry != nil {
		t.Fatalf("remove registry: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPatch, item, `{"schemaRegistry":{"url":"ftp://x"}}`, map[string]string{"If-Match": `"3"`})
	if resp.StatusCode != http.StatusUnprocessableEntity || !json.Valid(raw) || !containsPath(raw, "/schemaRegistry/url") {
		t.Fatalf("bad registry url: %d %s", resp.StatusCode, raw)
	}
}

func containsPath(raw []byte, path string) bool {
	var p struct {
		Path string `json:"path"`
	}
	return json.Unmarshal(raw, &p) == nil && p.Path == path
}
