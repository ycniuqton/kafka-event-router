package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// RouteScope identifies the route version a sandbox serves; pools never cross scopes.
// Formats are the source and destination formats; they select the boundary Marshaller.
// LookupPins are the library lookup tables (at their pinned versions) ctx.lookup can read.
type RouteScope struct {
	RouteID     string
	VersionID   string
	TeamID      string
	Parallelism int
	Mode        ScopeMode
	Formats     []boundary.Format
	LookupPins  []host.LookupPin
}

// Invocation is one event handed to a Module; SeedOverride replays a stored seed and Pin (test
// mode only, also accepted through determinism.PinContext) replays a saved test case.
type Invocation struct {
	Event        boundary.GuestEvent
	Now          time.Time
	SeedOverride *uint64
	Pin          *determinism.Pin
}

// Result is the outcome of one successful call.
type Result struct {
	Outputs       []boundary.Output
	Dropped       bool
	Bool          bool
	Logs          []string
	LogsTruncated bool
	Duration      time.Duration
	CPUTime       time.Duration
	MemoryBytes   int64
	Seed          uint64
	Determinism   determinism.Version
}

// Module runs one JS step or expr node of a route version; safe for concurrent calls.
type Module interface {
	Call(ctx context.Context, inv Invocation) (Result, error)
}

// RouteSandbox holds every Module of one route version. Modules are keyed by content
// (ModuleKey), so identical JS steps of different branches share one pool.
type RouteSandbox interface {
	Attach(ctx context.Context, src Source) (Module, error)
	Module(stepPath string) (Module, bool)
	Stats() []PoolStats
	Usage() RouteUsage
	Prewarm(ctx context.Context, target int) (PrewarmReport, error)
	Close(ctx context.Context) error
}

// ModuleKey identifies a module by entry and code, independent of the branch it sits in.
type ModuleKey string

// ModuleKeyPart separates the hashed members of a ModuleKey.
type ModuleKeyPart string

const ModuleKeySeparator ModuleKeyPart = "\x00"

// ModuleKeyOf hashes the entry kind and code of src.
func ModuleKeyOf(src Source) ModuleKey {
	sum := sha256.Sum256([]byte(string(src.Entry) + string(ModuleKeySeparator) + src.Code))
	return ModuleKey(hex.EncodeToString(sum[:]))
}
