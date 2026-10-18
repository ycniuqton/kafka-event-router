package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// LogActor is the resolved actor of a stored entry (name joined from users).
type LogActor struct {
	ID   *uuid.UUID `json:"id"`
	Name string     `json:"name"`
	Kind ActorKind  `json:"kind"`
}

// LogEntry is one audit_log row as read back by the audit screen and the activity feed.
type LogEntry struct {
	ID          int64           `json:"id,string"`
	At          time.Time       `json:"at"`
	Actor       LogActor        `json:"actor"`
	Action      Action          `json:"action"`
	Category    Category        `json:"category"`
	ObjectType  ObjectType      `json:"objectType"`
	ObjectID    string          `json:"objectId"`
	ObjectLabel string          `json:"objectLabel"`
	TeamID      *uuid.UUID      `json:"teamId"`
	IP          string          `json:"ip"`
	RequestID   string          `json:"requestId"`
	Sensitive   bool            `json:"sensitive"`
	Diff        json.RawMessage `json:"diff,omitempty"`
}

// Page is one keyset page request (newest first).
type Page struct {
	Limit int
	After *store.Cursor
}

// EntryReader reads audit entries (Postgres implementation below; fakes in tests).
type EntryReader interface {
	Query(ctx context.Context, f Filter, page Page) ([]LogEntry, *store.Cursor, error)
}

// PostgresReader runs keyset queries on (at DESC, id DESC) with the actor name joined.
type PostgresReader struct {
	db        *store.DB
	selectSQL string
	where     WhereBuilder
}

// NewPostgresReader builds the reader; entry labels fall back to live object names (NewObjectLabeler).
func NewPostgresReader(db *store.DB) *PostgresReader {
	labeler := NewObjectLabeler()
	return &PostgresReader{db: db, selectSQL: fmt.Sprintf(selectEntriesSQL, labeler.Projection()), where: WhereBuilder{Search: labeler.Search()}}
}

// selectEntriesSQL is the projection every audit read shares (%s is the object label expression).
const selectEntriesSQL = `SELECT a.id, a.at, a.actor_id, COALESCE(u.display_name, ''), a.actor_kind, a.action, a.object_type,
	COALESCE(a.object_id, ''), %s, a.team_id, COALESCE(host(a.ip), ''), COALESCE(a.request_id, ''),
	a.sensitive, a.diff FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id`

// Query returns one page and the cursor of the next one (nil on the last page).
func (r *PostgresReader) Query(ctx context.Context, f Filter, page Page) ([]LogEntry, *store.Cursor, error) {
	where, args := r.where.Build(f)
	sql, args := store.KeysetQuery{Select: r.selectSQL, Where: where, Args: args, SortColumn: "a.at", SortCast: "timestamptz",
		IDColumn: "a.id", IDCast: "bigint", Limit: page.Limit, After: page.After}.Build()
	rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, r.db.Translate(err)
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		e, err := r.scan(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, r.db.Translate(err)
	}
	items, next := store.PageSlicer[LogEntry]{Limit: page.Limit, KeyOf: EntryCursor{}.Of}.Slice(out)
	return items, next, nil
}

func (r *PostgresReader) scan(row interface{ Scan(...any) error }) (LogEntry, error) {
	var e LogEntry
	var diff []byte
	err := row.Scan(&e.ID, &e.At, &e.Actor.ID, &e.Actor.Name, &e.Actor.Kind, &e.Action, &e.ObjectType, &e.ObjectID,
		&e.ObjectLabel, &e.TeamID, &e.IP, &e.RequestID, &e.Sensitive, &diff)
	e.Diff = diff
	return e, err
}

// EntryCursor derives keyset cursors from entries.
type EntryCursor struct{}

// Of returns the cursor positioned after e.
func (EntryCursor) Of(e LogEntry) store.Cursor {
	return store.Cursor{SortKey: []string{e.At.Format(time.RFC3339Nano)}, ID: fmt.Sprint(e.ID)}
}

// WhereBuilder renders a Filter into SQL predicates with positional arguments; Search is the free-text
// predicate on parameter `$%[1]d` (ObjectLabeler.Search; stored label, id and action when empty).
type WhereBuilder struct {
	Search string
}

// DefaultTextSearch matches the stored label, the object id and the action.
const DefaultTextSearch = "(a.object_label ILIKE $%[1]d OR a.object_id ILIKE $%[1]d OR a.action ILIKE $%[1]d)"

// LikeEscaper escapes LIKE wildcards in user text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Build returns the predicates and their arguments in order.
func (w WhereBuilder) Build(f Filter) ([]string, []any) {
	search := w.Search
	if search == "" {
		search = DefaultTextSearch
	}
	c := &clauses{}
	c.addIf(f.Actor != nil, "a.actor_id = $%d", func() any { return *f.Actor })
	c.addIf(len(f.Actions) > 0, "a.action = ANY($%d)", func() any { return ActionList(f.Actions).Strings() })
	c.addIf(f.ObjectType != nil, "a.object_type = $%d", func() any { return string(*f.ObjectType) })
	c.addIf(f.ObjectID != nil, "a.object_id = $%d", func() any { return *f.ObjectID })
	c.addIf(f.TeamID != nil, "a.team_id = $%d", func() any { return *f.TeamID })
	c.addIf(f.From != nil, "a.at >= $%d", func() any { return *f.From })
	c.addIf(f.To != nil, "a.at < $%d", func() any { return *f.To })
	if f.SensitiveOnly {
		c.where = append(c.where, "a.sensitive")
	}
	c.addIf(f.Text != "", search,
		func() any { return "%" + likeEscaper.Replace(f.Text) + "%" })
	return c.where, c.args
}

// clauses accumulates predicates; each format string takes the argument's position.
type clauses struct {
	where []string
	args  []any
}

func (c *clauses) addIf(cond bool, format string, arg func() any) {
	if !cond {
		return
	}
	c.args = append(c.args, arg())
	c.where = append(c.where, fmt.Sprintf(format, len(c.args)))
}

// ActionList converts typed actions for SQL array parameters.
type ActionList []Action

// Strings returns the raw action names.
func (l ActionList) Strings() []string {
	out := make([]string, len(l))
	for i, a := range l {
		out[i] = string(a)
	}
	return out
}
