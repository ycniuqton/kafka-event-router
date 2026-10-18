package routestore

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
)

func TestListFilterBuildsNumberedClauses(t *testing.T) {
	where, args := ListFilter{}.Build(routes.ListQuery{Search: "50%_off", Tag: "pii", ConnectionID: "c1", TeamIDs: []routes.TeamID{"t1"},
		Statuses: []routes.Status{routes.StatusRunning, routes.StatusPaused}})
	sql := strings.Join(where, " AND ")
	for _, want := range []string{"$1::text", "$2::text = ANY(r.tags)", "jsonb_build_object('c', $3::text)", "r.team_id::text = ANY($4::text[])", " OR "} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in %s", want, sql)
		}
	}
	if len(args) != 4 || args[0] != `50\%\_off` {
		t.Fatalf("%v", args)
	}
	where, _ = ListFilter{}.Build(routes.ListQuery{})
	if len(where) != 1 || where[0] != hideArchived {
		t.Fatal(where)
	}
}

func TestChangeSetOnlyTouchesPresentFields(t *testing.T) {
	name, by := "n", uuid.New()
	set, args := ChangeSet{}.Build(routes.RouteChange{Name: &name, Draft: json.RawMessage(`{}`)}, &by)
	if set != "name = $1, draft_config = $2::jsonb, updated_by = COALESCE($3, updated_by)" || len(args) != 3 {
		t.Fatalf("%s %v", set, args)
	}
}

func TestEveryStatusHasAPredicate(t *testing.T) {
	for _, s := range routes.AllStatuses {
		if statusPredicates[s] == "" {
			t.Errorf("no predicate for %s", s)
		}
	}
}
