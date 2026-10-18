package portability

import (
	"context"

	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
)

// ConnectionDirectory finds connections by id and by name (connections table; adapter in routestore).
type ConnectionDirectory interface {
	validation.ConnectionLookup
	// FindByNames resolves names, preferring connections of team, keyed by name.
	FindByNames(ctx context.Context, teamID string, names []string) (map[string]validation.ConnectionInfo, error)
}

// ConnectionResolver swaps connection ids for names on export and back on import.
type ConnectionResolver struct {
	Directory ConnectionDirectory
}

// ToNames replaces every known connection id by its name; unknown ids are left as they are.
func (r ConnectionResolver) ToNames(ctx context.Context, tree DocumentTree) error {
	values := tree.Values()
	if len(values) == 0 {
		return nil
	}
	found, err := r.Directory.Find(ctx, values)
	if err != nil {
		return err
	}
	for _, s := range tree.Slots() {
		if c, ok := found[s.Value()]; ok && c.Name != "" {
			s.Set(c.Name)
		}
	}
	return nil
}

// ToIDs replaces names by ids (team connections first); existing ids are kept, anything else is an issue.
func (r ConnectionResolver) ToIDs(ctx context.Context, teamID string, tree DocumentTree) (config.Issues, error) {
	values := tree.Values()
	if len(values) == 0 {
		return config.Issues{}, nil
	}
	byName, err := r.Directory.FindByNames(ctx, teamID, values)
	if err != nil {
		return nil, err
	}
	byID, err := r.Directory.Find(ctx, values)
	if err != nil {
		return nil, err
	}
	issues := config.Issues{}
	for _, s := range tree.Slots() {
		issues = append(issues, r.resolve(s, byName, byID)...)
	}
	return issues, nil
}

// resolve rewrites one slot or reports it as unknown.
func (ConnectionResolver) resolve(s ConnectionSlot, byName, byID map[string]validation.ConnectionInfo) config.Issues {
	if c, ok := byName[s.Value()]; ok {
		s.Set(c.ID)
		return nil
	}
	if _, ok := byID[s.Value()]; ok {
		return nil
	}
	return config.Issues{config.NewError(config.IssueConnectionNotFound, s.Path,
		"No connection named %q exists in your team. Create it or change the name in the file.", s.Value())}
}
