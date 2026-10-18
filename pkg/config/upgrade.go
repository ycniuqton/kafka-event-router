package config

import "fmt"

// Upgrader brings a document of one API version to the current in-memory model.
type Upgrader interface {
	From() APIVersion
	Upgrade(doc *Document) (*Document, error)
}

// UpgraderManager selects the upgrader by the document's apiVersion.
type UpgraderManager struct {
	upgraders map[APIVersion]Upgrader
}

// NewUpgraderManager registers the given upgraders.
func NewUpgraderManager(us ...Upgrader) *UpgraderManager {
	m := &UpgraderManager{upgraders: map[APIVersion]Upgrader{}}
	for _, u := range us {
		m.upgraders[u.From()] = u
	}
	return m
}

// DefaultUpgraderManager knows router/v1 only.
func DefaultUpgraderManager() *UpgraderManager {
	return NewUpgraderManager(IdentityUpgrader{Version: APIVersionRouterV1})
}

// ForVersion returns the upgrader for v or an error for unknown versions.
func (m *UpgraderManager) ForVersion(v APIVersion) (Upgrader, error) {
	u, ok := m.upgraders[v]
	if !ok {
		return nil, fmt.Errorf("unsupported apiVersion %q", v)
	}
	return u, nil
}

// IdentityUpgrader returns documents of the current version unchanged.
type IdentityUpgrader struct {
	Version APIVersion
}

// From returns the version handled.
func (u IdentityUpgrader) From() APIVersion { return u.Version }

// Upgrade returns doc unchanged.
func (IdentityUpgrader) Upgrade(doc *Document) (*Document, error) { return doc, nil }
