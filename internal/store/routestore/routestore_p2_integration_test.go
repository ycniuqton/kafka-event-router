package routestore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/routestore"
)

// TestP2LookupsAgainstPostgres: FindByName (import), multi-team list filter, connection name resolution
// preferring the importing team, and the schema registry flag (P2 formats).
func TestP2LookupsAgainstPostgres(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	r := f.route(t, "orders")
	got, err := f.repo.Routes.FindByName(ctx, f.team, "orders")
	if err != nil || got.ID != r.ID {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := f.repo.Routes.FindByName(ctx, f.team, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing name: %v", err)
	}
	other := f.scalar(t, `INSERT INTO teams (name) VALUES ('partners') RETURNING id::text`)
	rows, _, err := f.repo.Routes.ListSummaries(ctx, routes.ListQuery{TeamIDs: []routes.TeamID{routes.TeamID(other)}}, routes.PageRequest{Limit: 10})
	if err != nil || len(rows) != 0 {
		t.Fatalf("other team: %d %v", len(rows), err)
	}
	rows, _, _ = f.repo.Routes.ListSummaries(ctx, routes.ListQuery{TeamIDs: []routes.TeamID{routes.TeamID(other), f.team}}, routes.PageRequest{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("both teams: %d", len(rows))
	}
	mine := f.scalar(t, `INSERT INTO connections (team_id, name, env_tag, bootstrap_servers, security_mode, schema_registry)
		VALUES ($1, 'kafka', 'dev', ARRAY['k:9092'], 'plaintext', '{"url":"http://sr:8081"}') RETURNING id::text`, string(f.team))
	theirs := f.scalar(t, `INSERT INTO connections (team_id, name, env_tag, bootstrap_servers, security_mode)
		VALUES ($1, 'kafka', 'prod', ARRAY['k:9092'], 'plaintext') RETURNING id::text`, other)
	lookup := routestore.NewConnectionLookup(f.db)
	byName, err := lookup.FindByNames(ctx, string(f.team), []string{"kafka", "ghost"})
	if err != nil || len(byName) != 1 || byName["kafka"].ID != mine || !byName["kafka"].HasSchemaRegistry {
		t.Fatalf("%+v %v", byName, err)
	}
	byName, _ = lookup.FindByNames(ctx, other, []string{"kafka"})
	byID, _ := lookup.Find(ctx, []string{theirs})
	if byName["kafka"].ID != theirs || byID[theirs].HasSchemaRegistry {
		t.Fatalf("%+v %+v", byName, byID)
	}
}

func (f *repoFixture) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := f.db.Pool().QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
