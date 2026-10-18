package libraryhttp_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library/libraryhttp"
)

func TestOwnerTeamPolicy(t *testing.T) {
	viewerTeam, editorTeam := uuid.New(), uuid.New()
	p := auth.Principal{Teams: []auth.TeamRole{{TeamID: viewerTeam, Role: auth.RoleViewer}, {TeamID: editorTeam, Role: auth.RoleEditor}}}
	cases := []struct {
		name      string
		principal auth.Principal
		requested uuid.UUID
		want      uuid.UUID
		fails     bool
	}{
		{"first writable team", p, uuid.Nil, editorTeam, false},
		{"requested writable team", p, editorTeam, editorTeam, false},
		{"requested read-only team", p, viewerTeam, uuid.Nil, true},
		{"no team", auth.Principal{}, uuid.Nil, uuid.Nil, true},
	}
	for _, c := range cases {
		got, err := libraryhttp.OwnerTeamPolicy{}.Resolve(c.principal, c.requested)
		if got != c.want || (err != nil) != c.fails {
			t.Errorf("%s: got %s, %v", c.name, got, err)
		}
	}
}

func TestEveryKindHasRoutesAndActions(t *testing.T) {
	if n := len(libraryhttp.NewModule(libraryhttp.Deps{}).Handlers()); n != 9*len(libraryhttp.KindRouteTable)+1 {
		t.Fatalf("handlers = %d", n)
	}
	if n := len(libraryhttp.AuditActions()); n != 4*len(libraryhttp.KindRouteTable) {
		t.Fatalf("actions = %d", n)
	}
}

func TestListNames(t *testing.T) {
	got := (libraryhttp.ListNames{}).Of([]string{"a, b", "", "c"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("names = %v", got)
	}
	if (libraryhttp.ListNames{}).Of(nil) != nil {
		t.Fatal("no names must stay nil")
	}
}
