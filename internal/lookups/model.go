// Package lookups is the lookup-source domain (connections 00001-lookup-sources): CRUD of HTTP, Redis,
// Postgres and static sources with write-only secrets, usage guard, audit, the Test button and the
// internal spec resolution used by workers and previews.
package lookups

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/lookup"
)

// EnvTag reuses the connection environment tags (dev / staging / prod).
type EnvTag = connections.EnvTag

// HealthStatus reuses the connection health statuses.
type HealthStatus = connections.HealthStatus

// RouteRef names a route using a source.
type RouteRef = connections.RouteRef

// LastFailure explains the failed step of the last test.
type LastFailure struct {
	Step    lookup.ProbeStage `json:"step"`
	Message string            `json:"message"`
	Hint    string            `json:"hint"`
}

// Health is the result of the last test.
type Health struct {
	Status      HealthStatus `json:"status"`
	CheckedAt   *time.Time   `json:"checkedAt"`
	LastFailure *LastFailure `json:"lastFailure"`
}

// Source is a saved lookup source.
type Source struct {
	ID           uuid.UUID
	TeamID       uuid.UUID
	TeamName     string
	Name         string
	Description  string
	Kind         lookup.SourceKind
	EnvTag       EnvTag
	Sensitive    bool
	Settings     lookup.Settings
	Limits       lookup.Limits
	SecretRef    secrets.Ref
	SecretFields []lookup.SecretField
	Health       Health
	Version      store.RowVersion
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Spec builds the runtime spec with the decrypted secrets.
func (s Source) Spec(b lookup.SecretBundle) lookup.SourceSpec {
	return lookup.SourceSpec{ID: s.ID.String(), TeamID: s.TeamID.String(), Name: s.Name, Kind: s.Kind, Version: int64(s.Version), Limits: s.Limits, Secrets: b, Settings: s.Settings}
}

// Draft is the desired state of a source (create, merged update, or unsaved test).
type Draft struct {
	TeamID      uuid.UUID
	Name        string
	Description string
	Kind        lookup.SourceKind
	EnvTag      EnvTag
	Sensitive   bool
	Settings    lookup.Settings
	Limits      lookup.Limits
	Secrets     lookup.SecretBundle
}

// Spec builds the runtime spec of the draft (unsaved test).
func (d Draft) Spec() lookup.SourceSpec {
	return lookup.SourceSpec{Name: d.Name, Kind: d.Kind, Limits: d.Limits, Secrets: d.Secrets, Settings: d.Settings}
}

// ListQuery filters and pages the list.
type ListQuery struct {
	Kind  lookup.SourceKind
	Env   EnvTag
	Text  string
	Limit int
	After *store.Cursor
}

// Page is one page of sources.
type Page struct {
	Items []Source
	Next  *store.Cursor
}
