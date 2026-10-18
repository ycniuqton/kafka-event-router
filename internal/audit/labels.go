package audit

import (
	"fmt"
	"sort"
	"strings"
)

// LabelSource is the SQL scalar subquery that names one object type from the entry's object id (`a.object_id`).
type LabelSource string

// LabelFinder is the reverse of a LabelSource: the ids (as text) of live objects whose name matches the
// search parameter `$%[1]d`, so text search can use the audit_log object_id index instead of naming every row.
type LabelFinder string

// ObjectLabeler renders the projection that falls back to the live object name when the stored
// object_label is empty, so the audit screen shows names instead of ids (deleted objects stay unnamed).
type ObjectLabeler struct {
	sources map[ObjectType]LabelSource
	finders map[ObjectType]LabelFinder
}

// NewObjectLabeler returns the labeler with the built-in object types (routes, versions, deployments,
// connections, users, teams, transforms, lookup tables, sample sessions as "<connection> · <topics>").
func NewObjectLabeler() *ObjectLabeler {
	return &ObjectLabeler{sources: map[ObjectType]LabelSource{
		ObjectRoute:       `SELECT name FROM routes WHERE id::text = a.object_id`,
		ObjectConnection:  `SELECT name FROM connections WHERE id::text = a.object_id`,
		ObjectUser:        `SELECT display_name FROM users WHERE id::text = a.object_id`,
		ObjectTeam:        `SELECT name FROM teams WHERE id::text = a.object_id`,
		ObjectTransform:   `SELECT name FROM transforms WHERE id::text = a.object_id`,
		ObjectLookupTable: `SELECT name FROM lookup_tables WHERE id::text = a.object_id`,
		ObjectRouteVersion: `SELECT r.name || ' v' || v.version_no FROM route_versions v JOIN routes r ON r.id = v.route_id
			WHERE v.id::text = a.object_id`,
		ObjectDeployment: `SELECT r.name || ' v' || v.version_no FROM deployments d JOIN route_versions v ON v.id = d.version_id
			JOIN routes r ON r.id = d.route_id WHERE d.id::text = a.object_id`,
		ObjectSampleSession: `SELECT COALESCE((SELECT name FROM connections WHERE id::text = a.diff->>'connectionId'),
			CASE a.diff->>'mode' WHEN 'demo' THEN 'demo data' ELSE 'upload' END)
			|| COALESCE(' · ' || (SELECT string_agg(t, ', ') FROM jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(a.diff->'topics') = 'array' THEN a.diff->'topics' ELSE '[]'::jsonb END) t), '')`,
	}, finders: map[ObjectType]LabelFinder{
		ObjectRoute:       `SELECT id::text FROM routes WHERE name ILIKE $%[1]d`,
		ObjectConnection:  `SELECT id::text FROM connections WHERE name ILIKE $%[1]d`,
		ObjectUser:        `SELECT id::text FROM users WHERE display_name ILIKE $%[1]d`,
		ObjectTeam:        `SELECT id::text FROM teams WHERE name ILIKE $%[1]d`,
		ObjectTransform:   `SELECT id::text FROM transforms WHERE name ILIKE $%[1]d`,
		ObjectLookupTable: `SELECT id::text FROM lookup_tables WHERE name ILIKE $%[1]d`,
		ObjectRouteVersion: `SELECT v.id::text FROM route_versions v JOIN routes r ON r.id = v.route_id
			WHERE r.name || ' v' || v.version_no ILIKE $%[1]d`,
		ObjectDeployment: `SELECT d.id::text FROM deployments d JOIN route_versions v ON v.id = d.version_id
			JOIN routes r ON r.id = d.route_id WHERE r.name || ' v' || v.version_no ILIKE $%[1]d`,
	}}
}

// RegisterFinder adds or replaces the reverse lookup of an object type (types without one are searched row by row).
func (l *ObjectLabeler) RegisterFinder(t ObjectType, f LabelFinder) {
	l.finders[t] = f
}

// Register adds or replaces the label source of an object type (features with their own tables).
func (l *ObjectLabeler) Register(t ObjectType, src LabelSource) {
	l.sources[t] = src
}

// Projection returns the SQL expression of the entry label: stored label first, then the live name.
func (l *ObjectLabeler) Projection() string {
	types := make([]string, 0, len(l.sources))
	for t := range l.sources {
		types = append(types, string(t))
	}
	sort.Strings(types)
	var b strings.Builder
	b.WriteString("COALESCE(NULLIF(a.object_label, ''), CASE a.object_type")
	for _, t := range types {
		fmt.Fprintf(&b, " WHEN '%s' THEN (%s)", t, l.sources[ObjectType(t)])
	}
	b.WriteString(" END, '')")
	return b.String()
}

// Search returns the free-text predicate (parameter `$%[1]d`): stored label, object id or action match through
// trigram indexes; unlabeled entries match when their live object's name does (finders), or row by row for
// types without a finder.
func (l *ObjectLabeler) Search() string {
	finders, scanned := make([]string, 0, len(l.finders)), make([]string, 0)
	for _, t := range l.sortedTypes() {
		if f, ok := l.finders[t]; ok {
			finders = append(finders, "("+string(f)+")")
			continue
		}
		scanned = append(scanned, "'"+string(t)+"'")
	}
	live := "a.object_id = ANY(ARRAY(" + strings.Join(finders, " UNION ALL ") + "))"
	if len(scanned) > 0 {
		live += " OR (a.object_type IN (" + strings.Join(scanned, ", ") + ") AND " +
			strings.ReplaceAll(l.Projection(), "%", "%%") + " ILIKE $%[1]d)"
	}
	return "(a.object_label ILIKE $%[1]d OR a.object_id ILIKE $%[1]d OR a.action ILIKE $%[1]d" +
		" OR (COALESCE(a.object_label, '') = '' AND (" + live + ")))"
}

// sortedTypes lists every type with a label source or a finder in lexical order.
func (l *ObjectLabeler) sortedTypes() []ObjectType {
	seen := map[ObjectType]bool{}
	for t := range l.sources {
		seen[t] = true
	}
	for t := range l.finders {
		seen[t] = true
	}
	out := make([]ObjectType, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
