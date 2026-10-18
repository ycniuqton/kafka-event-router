package store

import (
	"context"
	"embed"
)

// Migrations embeds the versioned SQL migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// MigrationsDir is the directory inside Migrations holding the SQL files.
type MigrationsDir string

const MigrationsRoot MigrationsDir = "migrations"

// Page requests a cursor-paginated slice of results.
type Page struct {
	Cursor string
	Limit  int
}

// Repository is the base contract every entity repository implements.
type Repository[T any, ID comparable] interface {
	Get(ctx context.Context, id ID) (T, error)
	List(ctx context.Context, page Page) (items []T, next string, err error)
	Create(ctx context.Context, entity T) (T, error)
	Update(ctx context.Context, entity T) (T, error)
	Delete(ctx context.Context, id ID) error
}

// TxManager runs a unit of work inside one database transaction.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
