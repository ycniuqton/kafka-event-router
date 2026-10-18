package routes

import (
	"context"
	"strings"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// RouteManager creates, reads, lists, edits, duplicates and archives routes.
type RouteManager struct {
	*Core
}

// NewRouteManager builds the manager over the shared core.
func NewRouteManager(core *Core) *RouteManager {
	return &RouteManager{Core: core}
}

// List returns one page of summaries.
func (m *RouteManager) List(ctx context.Context, q ListQuery, page PageRequest) ([]RouteSummary, *store.Cursor, error) {
	rows, next, err := m.Repos.Routes.ListSummaries(ctx, q, page)
	if err != nil {
		return nil, nil, err
	}
	return m.Views.Summaries(ctx, rows), next, nil
}

// Get returns the detail view.
func (m *RouteManager) Get(ctx context.Context, id RouteID) (RouteDetail, error) {
	return m.detail(ctx, id)
}

// Create stores a new paused route with an optional draft (defaults applied) and lists draft issues.
func (m *RouteManager) Create(ctx context.Context, in NewRoute) (RouteDetail, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := (SettingsRules{}).Check(RouteChange{Name: &in.Name, Tags: &in.Tags, Draft: in.Draft}); err != nil {
		return RouteDetail{}, err
	}
	actor := ActorResolver{}.From(ctx)
	team, err := m.teamFor(actor, in.TeamID)
	if err != nil {
		return RouteDetail{}, err
	}
	draft, err := DraftEditor{}.WithDefaults(in.Draft, in.Name)
	if err != nil {
		return RouteDetail{}, err
	}
	route := Route{TeamID: team, Name: in.Name, Description: in.Description, Tags: NonNilTags(in.Tags),
		DesiredState: DesiredPaused, Replicas: int(DefaultReplica), Draft: draft, CreatedBy: actor.UserID, UpdatedBy: actor.UserID}
	return m.insert(ctx, route, ActionRouteCreated)
}

// teamFor resolves the owning team: the requested one if writable, else the caller's first writable team.
func (m *RouteManager) teamFor(actor Actor, requested TeamID) (TeamID, error) {
	if requested != "" {
		if !m.Guard.CanWrite(actor, requested) {
			return "", ErrForbiddenTeam
		}
		return requested, nil
	}
	team, ok := actor.DefaultTeam()
	if !ok {
		return "", ErrNoWritableTeam
	}
	return team, nil
}

// insert checks the name, writes the row and its audit entry in one transaction.
func (m *RouteManager) insert(ctx context.Context, route Route, action audit.Action) (RouteDetail, error) {
	var created Route
	err := m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		taken, err := m.Repos.Routes.NameTaken(ctx, route.TeamID, route.Name, "")
		if err != nil || taken {
			return NameTakenOr(err, taken)
		}
		if created, err = m.Repos.Routes.Create(ctx, route); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: action, Object: audit.ObjectRoute, ObjectID: string(created.ID), Team: created.TeamID, After: AuditViewOf(created)})
	})
	if err != nil {
		return RouteDetail{}, err
	}
	return m.withIssues(ctx, created)
}

// withIssues reads the detail and attaches the draft-mode issues.
func (m *RouteManager) withIssues(ctx context.Context, r Route) (RouteDetail, error) {
	d, err := m.detail(ctx, r.ID)
	if err != nil {
		return RouteDetail{}, err
	}
	d.Issues = m.draftIssues(ctx, r).Issues
	return d, nil
}

// Update saves settings and/or the draft under If-Match; the draft is always saved, issues are listed.
func (m *RouteManager) Update(ctx context.Context, id RouteID, change RouteChange, expected Revision) (RouteDetail, error) {
	if change.Name != nil {
		trimmed := strings.TrimSpace(*change.Name)
		change.Name = &trimmed
	}
	if err := (SettingsRules{}).Check(change); err != nil {
		return RouteDetail{}, err
	}
	before, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	if before.DesiredState == DesiredArchived {
		return RouteDetail{}, ErrArchived
	}
	var after Route
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if after, err = m.save(ctx, before, change, expected, actor); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: UpdateAction(change), Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(after)})
	})
	if err != nil {
		return RouteDetail{}, err
	}
	return m.withIssues(ctx, after)
}

// save checks a rename and writes the versioned update.
func (m *RouteManager) save(ctx context.Context, before Route, change RouteChange, expected Revision, actor Actor) (Route, error) {
	if change.Name != nil && *change.Name != before.Name {
		taken, err := m.Repos.Routes.NameTaken(ctx, before.TeamID, *change.Name, before.ID)
		if err != nil || taken {
			return Route{}, NameTakenOr(err, taken)
		}
	}
	if change.Draft != nil {
		draft, err := DraftEditor{}.KeepConsumerGroup(change.Draft, before.Draft, before.Name)
		if err != nil {
			return Route{}, err
		}
		change.Draft = draft
	}
	return m.Repos.Routes.SaveDraft(ctx, DraftWrite{ID: before.ID, Change: change, Expected: expected, UpdatedBy: actor.UserID})
}

// Duplicate copies a route (draft, else active config) under "<name>-copy…" as a new paused draft.
func (m *RouteManager) Duplicate(ctx context.Context, id RouteID, name string) (RouteDetail, error) {
	src, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	name, err = m.copyName(ctx, src, strings.TrimSpace(name))
	if err != nil {
		return RouteDetail{}, err
	}
	doc, err := m.copySource(ctx, src)
	if err != nil {
		return RouteDetail{}, err
	}
	if doc, err = (DraftEditor{}).Retarget(doc, name); err != nil {
		return RouteDetail{}, err
	}
	copyRoute := Route{TeamID: src.TeamID, Name: name, Description: src.Description, Tags: NonNilTags(src.Tags),
		DesiredState: DesiredPaused, Replicas: src.Replicas, Draft: doc, CreatedBy: actor.UserID, UpdatedBy: actor.UserID}
	return m.insert(ctx, copyRoute, ActionRouteDuplicated)
}

// copyName validates a requested name or picks the next free "-copy" name.
func (m *RouteManager) copyName(ctx context.Context, src Route, requested string) (string, error) {
	if requested != "" {
		return requested, (SettingsRules{}).Check(RouteChange{Name: &requested})
	}
	taken, err := m.Repos.Routes.NamesWithPrefix(ctx, src.TeamID, DuplicateNamer{}.Prefix(src.Name))
	if err != nil {
		return "", err
	}
	return DuplicateNamer{}.Next(src.Name, taken), nil
}

// copySource is the draft, or the live config when the route has no draft.
func (m *RouteManager) copySource(ctx context.Context, src Route) ([]byte, error) {
	if src.Draft != nil || src.ActiveVersionID == nil {
		return src.Draft, nil
	}
	v, err := m.Repos.Versions.GetByID(ctx, *src.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	return v.Config, nil
}

// Archive stops and hides a route; archiving an archived route is a no-op.
func (m *RouteManager) Archive(ctx context.Context, id RouteID) error {
	before, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return err
	}
	if before.DesiredState == DesiredArchived {
		return m.Auditor.Record(ctx, AuditRecord{Action: ActionRouteArchived, Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(before)})
	}
	var after Route
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if after, err = m.Repos.Routes.SetDesiredState(ctx, id, DesiredArchived, actor.UserID); err != nil {
			return err
		}
		if err := m.Repos.Deployments.SupersedeOpen(ctx, id, SupersedeAllOpen()); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: ActionRouteArchived, Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(after)})
	})
	if err != nil {
		return err
	}
	m.publish(ctx, ChangeOf(after, ChangeArchived))
	return nil
}

// AddTag adds one tag (bulk action); existing tags are kept and duplicates ignored.
func (m *RouteManager) AddTag(ctx context.Context, id RouteID, tag string) (RouteDetail, error) {
	tag = strings.TrimSpace(tag)
	if err := (SettingsRules{}).Check(RouteChange{Tags: &[]string{tag}}); err != nil {
		return RouteDetail{}, err
	}
	before, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		after, err := m.Repos.Routes.AddTag(ctx, id, tag, actor.UserID)
		if err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: ActionRouteTagged, Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(after)})
	})
	if err != nil {
		return RouteDetail{}, err
	}
	return m.detail(ctx, id)
}

// UpdateAction is the audit action of a PATCH: draft saves are told apart from settings edits.
func UpdateAction(c RouteChange) audit.Action {
	if c.Draft != nil {
		return ActionRouteDraftSaved
	}
	return ActionRouteUpdated
}

// NameTakenOr returns err, or ErrNameTaken when the name is used.
func NameTakenOr(err error, taken bool) error {
	if err != nil {
		return err
	}
	if taken {
		return ErrNameTaken
	}
	return nil
}
