package host

import (
	"context"
	"time"
)

// Masker hides a value in one style (same implementation as the mask recipe).
type Masker interface {
	Mask(v any, style MaskStyle) (any, error)
}

// Hasher returns a salted hash of a value (salt bound per team).
type Hasher interface {
	Hash(ctx context.Context, v any) (string, error)
}

// DateFormatter renders a time in a named layout and IANA zone.
type DateFormatter interface {
	Format(t time.Time, layout string, tz string) (string, error)
}

// LookupResolver resolves a key in a named lookup table.
type LookupResolver interface {
	Lookup(table, key string) (any, bool)
}

// HostServices is implemented by the engine so recipe steps and ctx helpers share code.
type HostServices interface {
	Masker() Masker
	Hasher() Hasher
	DateFormatter() DateFormatter
	Lookups() LookupResolver
}

// ServicesBinder builds the services bound to one team's hash salt (called at Load).
type ServicesBinder interface {
	Bind(ctx context.Context, teamID string, salt []byte) (HostServices, error)
}
