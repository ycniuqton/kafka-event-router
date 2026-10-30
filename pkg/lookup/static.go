package lookup

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// TableLoader loads pinned library lookup tables (config.LibraryResolver satisfies it).
type TableLoader interface {
	LookupTable(ctx context.Context, ref string, version int) (config.LookupRows, error)
}

// ErrNoTables is returned when a static source is used where no library is wired.
var ErrNoTables = errors.New("no library lookup tables are available here")

// ErrEmptyTable is the probe failure of a table without rows.
var ErrEmptyTable = errors.New("lookup table has no rows")

// StaticFactory builds static clients over the library.
type StaticFactory struct {
	Tables TableLoader
}

func (StaticFactory) Kind() SourceKind { return KindStatic }

func (f StaticFactory) New(spec SourceSpec) (Client, error) {
	if spec.Static == nil {
		return nil, fmt.Errorf("static settings missing")
	}
	return &StaticClient{settings: *spec.Static, tables: f.Tables}, nil
}

// StaticClient answers keys from one pinned library table, loaded once (a failed load is retried).
type StaticClient struct {
	settings StaticSettings
	tables   TableLoader
	mu       sync.Mutex
	rows     map[string]any
}

// Fetch implements Client.
func (c *StaticClient) Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	rows, err := c.load(ctx)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, err
	}
	v, found := rows[req.Key]
	return ctxhelpers.SourceAnswer{Found: found, Value: v, Mode: ctxhelpers.AnswerLive}, nil
}

func (c *StaticClient) load(ctx context.Context) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows != nil {
		return c.rows, nil
	}
	if c.tables == nil {
		return nil, Failures{}.Permanent(ReasonUnavailable, ErrNoTables)
	}
	loaded, err := c.tables.LookupTable(ctx, c.settings.TableRef, c.settings.Version)
	if errors.Is(err, config.ErrLibraryRefMissing) {
		return nil, Failures{}.Permanent(ReasonUnknown, err)
	}
	if err != nil {
		return nil, Failures{}.Transient(ReasonUnavailable, err)
	}
	rows := make(map[string]any, len(loaded))
	for _, r := range loaded {
		rows[r.Key] = codec.NumberPolicy{}.Normalized(r.Value)
	}
	c.rows = rows
	return rows, nil
}

// Probe loads the table: a load failure fails reachable, an empty table fails sample.
func (c *StaticClient) Probe(ctx context.Context) error {
	rows, err := c.load(ctx)
	if err != nil {
		return &ProbeError{Stage: StageReachable, Hint: string(HintTable), Cause: err}
	}
	if len(rows) == 0 {
		return &ProbeError{Stage: StageSample, Hint: string(HintTable), Cause: ErrEmptyTable}
	}
	return nil
}

// Close implements Client.
func (c *StaticClient) Close() {}
