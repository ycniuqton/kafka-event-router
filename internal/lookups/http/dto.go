package lookupshttp

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/pkg/lookup"
)

// OwnerDTO names the owner team.
type OwnerDTO struct {
	TeamID   uuid.UUID `json:"teamId"`
	TeamName string    `json:"teamName"`
}

// OwnerInput is the requested owner team of a new source.
type OwnerInput struct {
	TeamID uuid.UUID `json:"teamId"`
}

// UsedByDTO lists the routes using a source.
type UsedByDTO struct {
	Count  int                `json:"count"`
	Routes []lookups.RouteRef `json:"routes"`
}

// SourceDTO is the public view of a source (design §3); settings of other kinds are null, secrets never appear.
type SourceDTO struct {
	ID           uuid.UUID                `json:"id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	Kind         lookup.SourceKind        `json:"kind"`
	EnvTag       lookups.EnvTag           `json:"envTag"`
	Sensitive    bool                     `json:"sensitive"`
	Owner        OwnerDTO                 `json:"owner"`
	HTTP         *lookup.HTTPSettings     `json:"http"`
	Redis        *lookup.RedisSettings    `json:"redis"`
	Postgres     *lookup.PostgresSettings `json:"postgres"`
	Static       *lookup.StaticSettings   `json:"static"`
	Limits       lookup.Limits            `json:"limits"`
	SecretFields []lookup.SecretField     `json:"secretFields"`
	Health       lookups.Health           `json:"health"`
	UsedBy       UsedByDTO                `json:"usedBy"`
	Version      int64                    `json:"version"`
	CreatedAt    time.Time                `json:"createdAt"`
	UpdatedAt    time.Time                `json:"updatedAt"`
}

// SourceInput is the create / draft-test body and the merge-patch target.
type SourceInput struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Kind        lookup.SourceKind        `json:"kind"`
	EnvTag      lookups.EnvTag           `json:"envTag"`
	Sensitive   bool                     `json:"sensitive"`
	Owner       *OwnerInput              `json:"owner,omitempty"`
	HTTP        *lookup.HTTPSettings     `json:"http"`
	Redis       *lookup.RedisSettings    `json:"redis"`
	Postgres    *lookup.PostgresSettings `json:"postgres"`
	Static      *lookup.StaticSettings   `json:"static"`
	Limits      *lookup.Limits           `json:"limits,omitempty"`
	Secrets     *lookup.SecretBundle     `json:"secrets,omitempty"`
}

// Draft converts the input for team (absent limits get the defaults).
func (in SourceInput) Draft(team uuid.UUID) lookups.Draft {
	limits := lookup.DefaultLimits()
	if in.Limits != nil {
		limits = *in.Limits
	}
	d := lookups.Draft{TeamID: team, Name: in.Name, Description: in.Description, Kind: in.Kind, EnvTag: in.EnvTag, Sensitive: in.Sensitive,
		Settings: lookup.Settings{HTTP: in.HTTP, Redis: in.Redis, Postgres: in.Postgres, Static: in.Static}, Limits: limits}
	if in.Secrets != nil {
		d.Secrets = *in.Secrets
	}
	return d
}

// InputOf is the merge-patch base of a stored source (secrets are never part of it).
func InputOf(s lookups.Source) SourceInput {
	limits := s.Limits
	return SourceInput{Name: s.Name, Description: s.Description, Kind: s.Kind, EnvTag: s.EnvTag, Sensitive: s.Sensitive,
		HTTP: s.Settings.HTTP, Redis: s.Settings.Redis, Postgres: s.Settings.Postgres, Static: s.Settings.Static, Limits: &limits}
}

// TestStepDTO is one checklist row.
type TestStepDTO struct {
	Step       lookup.ProbeStage `json:"step"`
	State      lookup.TestState  `json:"state"`
	DurationMs int64             `json:"durationMs"`
	Message    string            `json:"message,omitempty"`
	Hint       string            `json:"hint,omitempty"`
}

// TestResultDTO answers both test endpoints.
type TestResultDTO struct {
	OK        bool                 `json:"ok"`
	Health    lookups.HealthStatus `json:"health"`
	CheckedAt time.Time            `json:"checkedAt"`
	Steps     []TestStepDTO        `json:"steps"`
}

// Presenter renders domain values as DTOs.
type Presenter struct{}

// Source renders one source.
func (Presenter) Source(s lookups.Source, used UsedByDTO) SourceDTO {
	fields := s.SecretFields
	if fields == nil {
		fields = []lookup.SecretField{}
	}
	if used.Routes == nil {
		used.Routes = []lookups.RouteRef{}
	}
	return SourceDTO{ID: s.ID, Name: s.Name, Description: s.Description, Kind: s.Kind, EnvTag: s.EnvTag, Sensitive: s.Sensitive,
		Owner: OwnerDTO{TeamID: s.TeamID, TeamName: s.TeamName}, HTTP: s.Settings.HTTP, Redis: s.Settings.Redis,
		Postgres: s.Settings.Postgres, Static: s.Settings.Static, Limits: s.Limits, SecretFields: fields, Health: s.Health,
		UsedBy: used, Version: int64(s.Version), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

// Test renders a test result.
func (Presenter) Test(r lookups.TestResult) TestResultDTO {
	out := TestResultDTO{OK: r.OK, Health: r.Health, CheckedAt: r.CheckedAt, Steps: make([]TestStepDTO, 0, len(r.Steps))}
	for _, s := range r.Steps {
		out.Steps = append(out.Steps, TestStepDTO{Step: s.Stage, State: s.State, DurationMs: s.Duration.Milliseconds(), Message: s.Message, Hint: s.Hint})
	}
	return out
}
