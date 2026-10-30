package lookups_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/lookups/lookupstest"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/lookup"
)

func routeWith(sourceID, requestKind string) *config.RouteConfig {
	raw := fmt.Sprintf(`{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[{"kind":"if_else","condition":{"type":"cond","field":"value.a","op":"exists"},
 "thenSteps":[{"kind":"lookup","lookup":{"sourceId":%q,"request":{"kind":%q,"key":"k","op":"get","path":"/","sql":"SELECT 1"},"map":[{"from":"$","to":"value.x"}]}}]}]},
 "destination":{"connectionId":"c","topic":"out"}}]}`, sourceID, requestKind)
	doc, err := config.Parse([]byte(raw))
	if err != nil {
		panic(err)
	}
	cfg, _ := doc.Config()
	return cfg
}

func TestSourceRefValidator(t *testing.T) {
	repo := lookupstest.NewMemRepo()
	team := uuid.New()
	src, _ := repo.Create(context.Background(), lookups.Source{TeamID: team, Name: "flags", Kind: lookup.KindRedis})
	v := lookups.SourceRefValidator{Repo: repo}
	cases := []struct {
		id, kind, team string
		code           config.IssueCode
	}{
		{id: src.ID.String(), kind: "redis", team: team.String()},
		{id: uuid.NewString(), kind: "redis", team: team.String(), code: config.IssueConnectionNotFound},
		{id: "not-a-uuid", kind: "redis", team: team.String(), code: config.IssueConnectionNotFound},
		{id: src.ID.String(), kind: "redis", team: uuid.NewString(), code: config.IssueConnectionForbidden},
		{id: src.ID.String(), kind: "http", team: team.String(), code: config.IssueSchemaInvalid},
	}
	for _, c := range cases {
		issues, err := v.Validate(context.Background(), validation.ValidationInput{TeamID: c.team, Config: routeWith(c.id, c.kind)})
		if err != nil {
			t.Fatal(err)
		}
		if c.code == "" && len(issues) != 0 || c.code != "" && (len(issues) != 1 || issues[0].Code != c.code) {
			t.Fatalf("%+v: issues %+v", c, issues)
		}
		if len(issues) == 1 && issues[0].Path != "/branches/0/transform/steps/0/thenSteps/0/lookup/sourceId" {
			t.Fatalf("path %s", issues[0].Path)
		}
	}
}
