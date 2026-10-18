package config

import (
	"context"
	"errors"
)

// ErrLibraryRefMissing reports a library reference to an unknown ref or version.
var ErrLibraryRefMissing = errors.New("library reference not found")

// LibraryResolver resolves pinned library transforms and lookup tables at compile time
// (implemented by transform-studio, P3). External-data ports live in internal/routes/validation.
type LibraryResolver interface {
	Transform(ctx context.Context, ref string, version int) (Steps, error)
	LookupTable(ctx context.Context, ref string, version int) (LookupRows, error)
}
