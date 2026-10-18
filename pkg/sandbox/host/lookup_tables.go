package host

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// LookupPin pins one library lookup table to the version a route version was deployed with.
type LookupPin struct {
	Table   string `json:"table"`
	Version int    `json:"version"`
}

// LookupRows are the rows of one table version: key text → value.
type LookupRows map[string]any

// LibraryTables is the port to versioned library lookup tables (transform-studio's library store).
type LibraryTables interface {
	LookupTable(ctx context.Context, table string, version int) (LookupRows, error)
}

// ErrNoLibrary is returned when a route pins tables but the service has no library port.
var ErrNoLibrary = errors.New("lookup table library is not available in this service")

// NoLibraryTables fails every pinned table (services without library access).
type NoLibraryTables struct{}

func (NoLibraryTables) LookupTable(context.Context, string, int) (LookupRows, error) {
	return nil, ErrNoLibrary
}

// LookupLimit bounds ctx.lookup arguments so user code cannot make the host hash huge strings.
type LookupLimit int

const (
	LookupMaxTableBytes LookupLimit = 256
	LookupMaxKeyBytes   LookupLimit = 4096
)

// PinnedTables holds the library tables of one route version, loaded once before any event; it is
// immutable, so later library versions never change a running route.
type PinnedTables struct {
	tables map[string]LookupRows
	pins   []LookupPin
}

// Lookup resolves key in a pinned table.
func (p *PinnedTables) Lookup(table, key string) (any, bool) {
	v, ok := p.tables[table][key]
	return v, ok
}

// Pins lists the loaded pins in table order.
func (p *PinnedTables) Pins() []LookupPin { return p.pins }

// PinnedTablesLoader loads the pinned versions of a route version from the library.
type PinnedTablesLoader struct {
	Library LibraryTables
}

// Load fetches every pin; two pins of one table with different versions are a conflict.
func (l PinnedTablesLoader) Load(ctx context.Context, pins []LookupPin) (*PinnedTables, error) {
	out := &PinnedTables{tables: map[string]LookupRows{}}
	seen := map[string]int{}
	for _, pin := range pins {
		if v, dup := seen[pin.Table]; dup {
			if v != pin.Version {
				return nil, fmt.Errorf(string(MsgPinConflict), pin.Table, v, pin.Version)
			}
			continue
		}
		rows, err := l.Library.LookupTable(ctx, pin.Table, pin.Version)
		if err != nil {
			return nil, fmt.Errorf(string(MsgPinMissing), pin.Table, pin.Version, err)
		}
		seen[pin.Table], out.tables[pin.Table] = pin.Version, rows
		out.pins = append(out.pins, pin)
	}
	sort.Slice(out.pins, func(i, j int) bool { return out.pins[i].Table < out.pins[j].Table })
	return out, nil
}

// LayeredLookups asks the route's pinned tables first, then the services' own resolver.
type LayeredLookups struct {
	Pinned LookupResolver
	Base   LookupResolver
}

func (l LayeredLookups) Lookup(table, key string) (any, bool) {
	if v, ok := l.Pinned.Lookup(table, key); ok {
		return v, true
	}
	return l.Base.Lookup(table, key)
}

// PinnedServices decorates host services with a route version's pinned lookup tables.
type PinnedServices struct {
	HostServices
	Tables *PinnedTables
}

// Lookups layers the pinned tables over the base resolver.
func (s PinnedServices) Lookups() LookupResolver {
	return LayeredLookups{Pinned: s.Tables, Base: s.HostServices.Lookups()}
}
