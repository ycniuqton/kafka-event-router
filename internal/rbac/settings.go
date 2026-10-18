package rbac

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// WorkspaceSettingKey names one workspace-wide setting row (workspace_settings.key).
type WorkspaceSettingKey string

const (
	SettingAIAssistEnabled            WorkspaceSettingKey = "ai_assist_enabled"
	SettingDefaultRequireProdApproval WorkspaceSettingKey = "default_require_prod_approval"
)

// WorkspaceSettings is the typed view of every workspace setting plus the aggregate version (ETag).
type WorkspaceSettings struct {
	AIAssistEnabled            bool             `json:"aiAssistEnabled"`
	DefaultRequireProdApproval bool             `json:"defaultRequireProdApproval"`
	RowVersion                 store.RowVersion `json:"rowVersion"`
}

// SettingsPatch is PATCH /workspace-settings: nil members stay unchanged.
type SettingsPatch struct {
	AIAssistEnabled            *bool `json:"aiAssistEnabled"`
	DefaultRequireProdApproval *bool `json:"defaultRequireProdApproval"`
}

// SettingDefinition binds one key to its default and to its field in the typed view and in a patch.
type SettingDefinition struct {
	Key     WorkspaceSettingKey
	Default bool
	Field   func(*WorkspaceSettings) *bool
	Patched func(SettingsPatch) *bool
}

// SettingCatalog lists every workspace setting (AI assist is off until an admin enables it).
type SettingCatalog []SettingDefinition

// DefaultSettingCatalog returns the built-in settings (design §3: ai_assist_enabled, default_require_prod_approval).
func DefaultSettingCatalog() SettingCatalog {
	return SettingCatalog{
		{Key: SettingAIAssistEnabled, Default: false,
			Field:   func(s *WorkspaceSettings) *bool { return &s.AIAssistEnabled },
			Patched: func(p SettingsPatch) *bool { return p.AIAssistEnabled }},
		{Key: SettingDefaultRequireProdApproval, Default: DefaultTeamDefaults.RequireProdApproval,
			Field:   func(s *WorkspaceSettings) *bool { return &s.DefaultRequireProdApproval },
			Patched: func(p SettingsPatch) *bool { return p.DefaultRequireProdApproval }},
	}
}

// Resolve builds the typed view from stored values; missing or malformed values fall back to defaults.
func (c SettingCatalog) Resolve(stored StoredSettings) WorkspaceSettings {
	out := WorkspaceSettings{RowVersion: stored.Version}
	for _, def := range c {
		value := def.Default
		if raw, ok := stored.Values[def.Key]; ok {
			_ = json.Unmarshal(raw, &value)
		}
		*def.Field(&out) = value
	}
	return out
}

// StoredSettings are the raw rows and their aggregate version (1 + sum of row versions; grows on every change).
type StoredSettings struct {
	Values  map[WorkspaceSettingKey]json.RawMessage
	Version store.RowVersion
}

// SettingsStore persists workspace settings (Postgres below; fakes in tests).
type SettingsStore interface {
	Load(ctx context.Context) (StoredSettings, error)
	Lock(ctx context.Context) error
	Put(ctx context.Context, key WorkspaceSettingKey, value json.RawMessage, by *uuid.UUID) error
}

// SettingsLockKey is the transaction advisory lock serialising settings changes (7001 migrate, 7003 admins).
type SettingsLockKey int64

const WorkspaceSettingsLock SettingsLockKey = 7010

// PostgresSettingsStore reads and writes workspace_settings.
type PostgresSettingsStore struct {
	db *store.DB
}

// NewPostgresSettingsStore builds the store.
func NewPostgresSettingsStore(db *store.DB) *PostgresSettingsStore {
	return &PostgresSettingsStore{db: db}
}

// Load returns every stored row and the aggregate version.
func (s *PostgresSettingsStore) Load(ctx context.Context) (StoredSettings, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT key, value, row_version FROM workspace_settings`)
	if err != nil {
		return StoredSettings{}, s.db.Translate(err)
	}
	defer rows.Close()
	out := StoredSettings{Values: map[WorkspaceSettingKey]json.RawMessage{}, Version: 1}
	for rows.Next() {
		var key string
		var value []byte
		var version int64
		if err := rows.Scan(&key, &value, &version); err != nil {
			return StoredSettings{}, err
		}
		out.Values[WorkspaceSettingKey(key)] = value
		out.Version += store.RowVersion(version)
	}
	return out, s.db.Translate(rows.Err())
}

// Lock serialises concurrent changes for the rest of the transaction.
func (s *PostgresSettingsStore) Lock(ctx context.Context) error {
	_, err := s.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(WorkspaceSettingsLock))
	return s.db.Translate(err)
}

// Put inserts or updates one setting (the update trigger bumps its row_version).
func (s *PostgresSettingsStore) Put(ctx context.Context, key WorkspaceSettingKey, value json.RawMessage, by *uuid.UUID) error {
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO workspace_settings (key, value, updated_by) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by`, string(key), value, by)
	return s.db.Translate(err)
}
