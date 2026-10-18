package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MigrationLock is the advisory lock id serialising concurrent migrators across api replicas.
type MigrationLock int64

const MigratorLockID MigrationLock = 7001

// MigrationScheme maps the configured postgres URL scheme to the golang-migrate pgx v5 driver.
type MigrationScheme string

const (
	SchemePostgres   MigrationScheme = "postgres://"
	SchemePostgreSQL MigrationScheme = "postgresql://"
	SchemePgx5       MigrationScheme = "pgx5://"
)

// MigrationFileSuffix identifies up migrations inside the embedded directory.
type MigrationFileSuffix string

const (
	SuffixUp   MigrationFileSuffix = ".up.sql"
	SuffixDown MigrationFileSuffix = ".down.sql"
)

// MigrationStatus reports the applied and the latest embedded schema versions.
type MigrationStatus struct {
	Applied uint
	Latest  uint
	Dirty   bool
}

// ErrSchemaNewer is returned when the database was migrated by a newer binary.
var ErrSchemaNewer = errors.New("store: schema version newer than binary")

// Migrator applies the embedded migrations under advisory lock 7001.
type Migrator struct {
	db  *DB
	url string
	fs  fs.FS
}

// NewMigrator builds a Migrator for the given pool and database URL.
func NewMigrator(db *DB, databaseURL string) *Migrator {
	return &Migrator{db: db, url: databaseURL, fs: Migrations}
}

// Up applies all pending migrations; it refuses to run against a newer schema.
func (m *Migrator) Up(ctx context.Context) error {
	return m.locked(ctx, func(mg *migrate.Migrate) error {
		if err := m.ensureNotNewer(mg); err != nil {
			return err
		}
		return m.ignoreNoChange(mg.Up())
	})
}

// Down reverts the last n migrations.
func (m *Migrator) Down(ctx context.Context, n int) error {
	return m.locked(ctx, func(mg *migrate.Migrate) error {
		return m.ignoreNoChange(mg.Steps(-n))
	})
}

// Status returns the applied version, the latest embedded version and the dirty flag.
func (m *Migrator) Status(ctx context.Context) (MigrationStatus, error) {
	var st MigrationStatus
	err := m.locked(ctx, func(mg *migrate.Migrate) error {
		applied, dirty, err := m.applied(mg)
		st = MigrationStatus{Applied: applied, Dirty: dirty, Latest: m.Latest()}
		return err
	})
	return st, err
}

// Latest returns the highest version among the embedded up migrations.
func (m *Migrator) Latest() uint {
	entries, _ := fs.ReadDir(m.fs, string(MigrationsRoot))
	var latest uint
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.ParseUint(prefix, 10, 32)
		if ok && err == nil && strings.HasSuffix(e.Name(), string(SuffixUp)) && uint(v) > latest {
			latest = uint(v)
		}
	}
	return latest
}

func (m *Migrator) locked(ctx context.Context, fn func(*migrate.Migrate) error) error {
	conn, err := m.db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("store: migrator acquire: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(MigratorLockID)); err != nil {
		return fmt.Errorf("store: migrator lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", int64(MigratorLockID))
	}()
	mg, err := m.open()
	if err != nil {
		return err
	}
	defer func() { _, _ = mg.Close() }()
	return fn(mg)
}

func (m *Migrator) open() (*migrate.Migrate, error) {
	src, err := iofs.New(m.fs, string(MigrationsRoot))
	if err != nil {
		return nil, fmt.Errorf("store: migration source: %w", err)
	}
	mg, err := migrate.NewWithSourceInstance("iofs", src, m.driverURL())
	if err != nil {
		return nil, fmt.Errorf("store: migrator: %w", err)
	}
	return mg, nil
}

func (m *Migrator) driverURL() string {
	for _, scheme := range []MigrationScheme{SchemePostgres, SchemePostgreSQL} {
		if rest, ok := strings.CutPrefix(m.url, string(scheme)); ok {
			return string(SchemePgx5) + rest
		}
	}
	return m.url
}

func (m *Migrator) applied(mg *migrate.Migrate) (uint, bool, error) {
	v, dirty, err := mg.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, dirty, err
}

func (m *Migrator) ensureNotNewer(mg *migrate.Migrate) error {
	applied, _, err := m.applied(mg)
	if err != nil {
		return err
	}
	if latest := m.Latest(); applied > latest {
		return fmt.Errorf("%w: schema version %d newer than binary %d", ErrSchemaNewer, applied, latest)
	}
	return nil
}

func (m *Migrator) ignoreNoChange(err error) error {
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
