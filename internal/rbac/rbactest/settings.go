package rbactest

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Settings is an in-memory rbac.SettingsStore with per-key versions, like workspace_settings.
type Settings struct {
	mu       sync.Mutex
	values   map[rbac.WorkspaceSettingKey]json.RawMessage
	versions map[rbac.WorkspaceSettingKey]store.RowVersion
	Locks    int
}

// NewSettings returns an empty store (every setting at its default).
func NewSettings() *Settings {
	return &Settings{values: map[rbac.WorkspaceSettingKey]json.RawMessage{}, versions: map[rbac.WorkspaceSettingKey]store.RowVersion{}}
}

// Load implements rbac.SettingsStore.
func (s *Settings) Load(context.Context) (rbac.StoredSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := rbac.StoredSettings{Values: map[rbac.WorkspaceSettingKey]json.RawMessage{}, Version: 1}
	for k, v := range s.values {
		out.Values[k] = v
		out.Version += s.versions[k]
	}
	return out, nil
}

// Lock implements rbac.SettingsStore.
func (s *Settings) Lock(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Locks++
	return nil
}

// Put implements rbac.SettingsStore.
func (s *Settings) Put(_ context.Context, key rbac.WorkspaceSettingKey, value json.RawMessage, _ *uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	s.versions[key]++
	return nil
}
