package store

import (
	"fmt"
	"strings"
)

// PageLimit bounds the page size of cursor pagination.
type PageLimit int

const (
	DefaultPageLimit PageLimit = 50
	MaxPageLimit     PageLimit = 200
)

// SortDirection is the keyset ordering direction.
type SortDirection string

const (
	SortDesc SortDirection = "DESC"
	SortAsc  SortDirection = "ASC"
)

// comparator returns the row-value operator that continues after the cursor for the direction.
func (d SortDirection) comparator() string {
	if d == SortAsc {
		return ">"
	}
	return "<"
}

// KeysetQuery builds `WHERE (sort, id) < ($n, $m) ORDER BY sort, id LIMIT limit+1` queries.
// Column names and casts come from code, never from user input.
type KeysetQuery struct {
	Select     string
	Where      []string
	Args       []any
	SortColumn string
	SortCast   string
	IDColumn   string
	IDCast     string
	Direction  SortDirection
	Limit      int
	After      *Cursor
}

// Build renders the SQL and its positional arguments.
func (q KeysetQuery) Build() (string, []any) {
	args := append([]any{}, q.Args...)
	where := append([]string{}, q.Where...)
	if q.After != nil && len(q.After.SortKey) > 0 {
		args = append(args, q.After.SortKey[0], q.After.ID)
		where = append(where, fmt.Sprintf("(%s, %s) %s ($%d::%s, $%d::%s)",
			q.SortColumn, q.IDColumn, q.direction().comparator(), len(args)-1, q.SortCast, len(args), q.IDCast))
	}
	var sb strings.Builder
	sb.WriteString(q.Select)
	if len(where) > 0 {
		sb.WriteString(" WHERE " + strings.Join(where, " AND "))
	}
	dir := string(q.direction())
	fmt.Fprintf(&sb, " ORDER BY %s %s, %s %s LIMIT %d", q.SortColumn, dir, q.IDColumn, dir, q.Limit+1)
	return sb.String(), args
}

func (q KeysetQuery) direction() SortDirection {
	if q.Direction == "" {
		return SortDesc
	}
	return q.Direction
}

// PageSlicer trims a limit+1 result set and derives the next cursor from the last kept row.
type PageSlicer[T any] struct {
	Limit int
	KeyOf func(T) Cursor
}

// Slice returns at most Limit rows and the cursor of the last one when more rows exist.
func (p PageSlicer[T]) Slice(rows []T) ([]T, *Cursor) {
	if len(rows) <= p.Limit {
		return rows, nil
	}
	kept := rows[:p.Limit]
	next := p.KeyOf(kept[len(kept)-1])
	return kept, &next
}
