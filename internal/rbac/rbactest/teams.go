package rbactest

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// TeamStore is an in-memory rbac.TeamStore; Usages simulates owned objects.
type TeamStore struct {
	Items  map[rbac.TeamID]rbac.Team
	Usages map[rbac.TeamID]rbac.TeamUsage
}

// NewTeamStore returns an empty team store.
func NewTeamStore() *TeamStore {
	return &TeamStore{Items: map[rbac.TeamID]rbac.Team{}, Usages: map[rbac.TeamID]rbac.TeamUsage{}}
}

func (t *TeamStore) List(context.Context) ([]rbac.Team, error) {
	out := []rbac.Team{}
	for _, team := range t.Items {
		out = append(out, team)
	}
	return out, nil
}

func (t *TeamStore) Get(_ context.Context, id rbac.TeamID) (rbac.Team, error) {
	team, ok := t.Items[id]
	if !ok {
		return rbac.Team{}, store.ErrNotFound
	}
	return team, nil
}

func (t *TeamStore) Insert(_ context.Context, name string, require bool) (rbac.Team, error) {
	team := rbac.Team{ID: uuid.New(), Name: name, RequireProdApproval: require, CreatedAt: time.Now(), RowVersion: 1}
	t.Items[team.ID] = team
	return team, nil
}

func (t *TeamStore) Update(_ context.Context, id rbac.TeamID, c rbac.TeamChange, expected store.RowVersion) error {
	team := t.Items[id]
	if team.RowVersion != expected {
		return &store.PreconditionFailedError{Current: team.RowVersion}
	}
	if c.Name != nil {
		team.Name = *c.Name
	}
	if c.RequireProdApproval != nil {
		team.RequireProdApproval = *c.RequireProdApproval
	}
	team.RowVersion++
	t.Items[id] = team
	return nil
}

func (t *TeamStore) Delete(_ context.Context, id rbac.TeamID) error {
	delete(t.Items, id)
	return nil
}

func (t *TeamStore) Usage(_ context.Context, id rbac.TeamID) (rbac.TeamUsage, error) {
	return t.Usages[id], nil
}

func (t *TeamStore) NameTaken(_ context.Context, name string, except rbac.TeamID) (bool, error) {
	for id, team := range t.Items {
		if id != except && strings.EqualFold(team.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

// Connections is an in-memory rbac.ConnectionDirectory.
type Connections map[uuid.UUID]rbac.ConnectionInfo

// Find implements rbac.ConnectionDirectory.
func (c Connections) Find(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]rbac.ConnectionInfo, error) {
	out := map[uuid.UUID]rbac.ConnectionInfo{}
	for _, id := range ids {
		if info, ok := c[id]; ok {
			out[id] = info
		}
	}
	return out, nil
}

// Grants is an in-memory rbac.GrantStore.
type Grants struct {
	Items       map[[2]uuid.UUID]rbac.ConnectionGrant
	Connections Connections
}

// NewGrants returns an empty grant store over a connection directory.
func NewGrants(conns Connections) *Grants {
	return &Grants{Items: map[[2]uuid.UUID]rbac.ConnectionGrant{}, Connections: conns}
}

func (g *Grants) List(_ context.Context, team rbac.TeamID) ([]rbac.ConnectionGrant, error) {
	out := []rbac.ConnectionGrant{}
	for k, v := range g.Items {
		if k[0] == team {
			out = append(out, v)
		}
	}
	return out, nil
}

func (g *Grants) Exists(_ context.Context, team rbac.TeamID, conn uuid.UUID) (bool, error) {
	_, ok := g.Items[[2]uuid.UUID{team, conn}]
	return ok, nil
}

func (g *Grants) Insert(_ context.Context, team rbac.TeamID, conn uuid.UUID, by *uuid.UUID) (bool, error) {
	key := [2]uuid.UUID{team, conn}
	if _, ok := g.Items[key]; ok {
		return false, nil
	}
	c := g.Connections[conn]
	g.Items[key] = rbac.ConnectionGrant{TeamID: team, ConnectionID: conn, ConnectionName: c.Name, OwnerTeamID: c.TeamID, EnvTag: c.EnvTag, GrantedBy: by, CreatedAt: time.Now()}
	return true, nil
}

func (g *Grants) Delete(_ context.Context, team rbac.TeamID, conn uuid.UUID) (bool, error) {
	key := [2]uuid.UUID{team, conn}
	_, ok := g.Items[key]
	delete(g.Items, key)
	return ok, nil
}

// Invitations is an in-memory rbac.InvitationStore over a World.
type Invitations struct{ W *World }

// EmailExists implements rbac.InvitationStore.
func (i Invitations) EmailExists(_ context.Context, email string) (bool, error) {
	for _, u := range i.W.Users {
		if strings.EqualFold(u.Email, email) {
			return true, nil
		}
	}
	return false, nil
}

// InsertInvited implements rbac.InvitationStore.
func (i Invitations) InsertInvited(_ context.Context, email, name string, _ *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	i.W.Users[id] = &rbac.User{ID: id, Email: email, DisplayName: name, Status: rbac.UserInvited, Teams: []rbac.UserTeam{}, RowVersion: 1}
	return id, nil
}

// Accept implements rbac.InvitationAcceptor: invited users become active once.
func (i Invitations) Accept(_ context.Context, id uuid.UUID) (bool, error) {
	u, ok := i.W.Users[id]
	if !ok || u.Status != rbac.UserInvited {
		return false, nil
	}
	u.Status = rbac.UserActive
	return true, nil
}
