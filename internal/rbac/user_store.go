package rbac

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// UserStatus is the lifecycle state shown in Settings → Members.
type UserStatus string

const (
	UserActive   UserStatus = "active"
	UserInvited  UserStatus = "invited"
	UserDisabled UserStatus = "disabled"
)

// UserTeam is one team membership of a user.
type UserTeam struct {
	TeamID     TeamID           `json:"teamId"`
	TeamName   string           `json:"teamName"`
	Role       Role             `json:"role"`
	RowVersion store.RowVersion `json:"rowVersion"`
}

// User is the administrable view of a users row.
type User struct {
	ID             uuid.UUID        `json:"id"`
	Email          string           `json:"email"`
	DisplayName    string           `json:"displayName"`
	WorkspaceAdmin bool             `json:"workspaceAdmin"`
	Status         UserStatus       `json:"status"`
	Teams          []UserTeam       `json:"teams"`
	LastLoginAt    *time.Time       `json:"lastLoginAt"`
	RowVersion     store.RowVersion `json:"rowVersion"`

	PasswordLogin      bool `json:"passwordLogin"`
	MustChangePassword bool `json:"mustChangePassword"`
}

// UserFilter narrows user lists.
type UserFilter struct {
	Text   string
	TeamID *TeamID
}

// Page is one keyset page request.
type Page struct {
	Limit int
	After *store.Cursor
}

// UserChange is the persisted part of a user update (nil members unchanged).
type UserChange struct {
	DisplayName    *string
	WorkspaceAdmin *bool
	Disabled       *bool
}

// UserStore persists users for administration.
type UserStore interface {
	List(ctx context.Context, f UserFilter, page Page) ([]User, *store.Cursor, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	Update(ctx context.Context, id uuid.UUID, c UserChange, expected store.RowVersion) error
}

// PostgresUserStore implements UserStore.
type PostgresUserStore struct {
	db *store.DB
}

// NewPostgresUserStore builds the store.
func NewPostgresUserStore(db *store.DB) *PostgresUserStore {
	return &PostgresUserStore{db: db}
}

// selectUsersSQL is the projection shared by user reads.
const selectUsersSQL = `SELECT u.id, u.email, u.display_name, u.is_workspace_admin, u.disabled_at IS NOT NULL,
	u.sso_subject IS NULL, u.row_version, p.user_id IS NOT NULL, COALESCE(p.must_change, false)
	FROM users u LEFT JOIN user_passwords p ON p.user_id = u.id`

// UserSort names the keyset columns of user lists (ordered by email).
type UserSort string

const (
	UserSortColumn UserSort = "u.email"
	UserSortCast   UserSort = "text"
	UserIDColumn   UserSort = "u.id"
	UserIDCast     UserSort = "uuid"
)

// List returns one page of users ordered by email, with their teams.
func (s *PostgresUserStore) List(ctx context.Context, f UserFilter, page Page) ([]User, *store.Cursor, error) {
	where, args := userWhere{}.Build(f)
	sql, args := store.KeysetQuery{Select: selectUsersSQL, Where: where, Args: args, SortColumn: string(UserSortColumn),
		SortCast: string(UserSortCast), IDColumn: string(UserIDColumn), IDCast: string(UserIDCast), Direction: store.SortAsc,
		Limit: page.Limit, After: page.After}.Build()
	users, err := s.query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	items, next := store.PageSlicer[User]{Limit: page.Limit, KeyOf: func(u User) store.Cursor {
		return store.Cursor{SortKey: []string{u.Email}, ID: u.ID.String()}
	}}.Slice(users)
	return items, next, s.attachTeams(ctx, items)
}

// Get returns one user with teams.
func (s *PostgresUserStore) Get(ctx context.Context, id uuid.UUID) (User, error) {
	users, err := s.query(ctx, selectUsersSQL+` WHERE u.id = $1`, id)
	if err != nil {
		return User{}, err
	}
	if len(users) == 0 {
		return User{}, store.ErrNotFound
	}
	return users[0], s.attachTeams(ctx, users)
}

// Update applies the change under optimistic concurrency (412 on stale version).
func (s *PostgresUserStore) Update(ctx context.Context, id uuid.UUID, c UserChange, expected store.RowVersion) error {
	set := `display_name = COALESCE($1, display_name), is_workspace_admin = COALESCE($2, is_workspace_admin),
		disabled_at = CASE WHEN $3::boolean IS NULL THEN disabled_at WHEN $3 THEN COALESCE(disabled_at, now()) ELSE NULL END`
	_, err := store.VersionedUpdate{Table: "users", IDColumn: "id"}.Exec(ctx, s.db, set,
		[]any{c.DisplayName, c.WorkspaceAdmin, c.Disabled}, id, expected)
	return err
}

func (s *PostgresUserStore) query(ctx context.Context, sql string, args ...any) ([]User, error) {
	rows, err := s.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var disabled, invited bool
		var version int64
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.WorkspaceAdmin, &disabled, &invited, &version,
			&u.PasswordLogin, &u.MustChangePassword); err != nil {
			return nil, err
		}
		u.Status, u.RowVersion, u.Teams = UserStatusOf{}.Derive(disabled, invited), store.RowVersion(version), []UserTeam{}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *PostgresUserStore) attachTeams(ctx context.Context, users []User) error {
	if len(users) == 0 {
		return nil
	}
	index := map[uuid.UUID]int{}
	ids := make([]uuid.UUID, len(users))
	for i, u := range users {
		index[u.ID], ids[i] = i, u.ID
	}
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT rb.user_id, rb.team_id, t.name, rb.role::text, rb.row_version FROM role_bindings rb
		JOIN teams t ON t.id = rb.team_id WHERE rb.user_id = ANY($1) ORDER BY t.name`, ids)
	if err != nil {
		return s.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid uuid.UUID
		var t UserTeam
		var role string
		var version int64
		if err := rows.Scan(&uid, &t.TeamID, &t.TeamName, &role, &version); err != nil {
			return err
		}
		t.Role, t.RowVersion = Role(role), store.RowVersion(version)
		users[index[uid]].Teams = append(users[index[uid]].Teams, t)
	}
	return rows.Err()
}

// UserStatusOf derives a user's status from stored flags.
type UserStatusOf struct{}

// Derive returns disabled before invited before active.
func (UserStatusOf) Derive(disabled, invited bool) UserStatus {
	if disabled {
		return UserDisabled
	}
	if invited {
		return UserInvited
	}
	return UserActive
}

// userWhere renders a UserFilter into predicates.
type userWhere struct{}

// likeEscaper escapes LIKE wildcards in user text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (userWhere) Build(f UserFilter) ([]string, []any) {
	var where []string
	var args []any
	if f.Text != "" {
		args = append(args, "%"+likeEscaper.Replace(f.Text)+"%")
		where = append(where, fmt.Sprintf("(u.email ILIKE $%[1]d OR u.display_name ILIKE $%[1]d)", len(args)))
	}
	if f.TeamID != nil {
		args = append(args, *f.TeamID)
		where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM role_bindings f WHERE f.user_id = u.id AND f.team_id = $%d)", len(args)))
	}
	return where, args
}
