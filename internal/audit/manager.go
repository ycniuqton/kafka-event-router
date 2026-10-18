package audit

import (
	"context"

	"github.com/eventrouter/router/internal/store"
)

// QueryResult is one page of entries and the cursor of the next page (nil when done).
type QueryResult struct {
	Items []LogEntry
	Next  *store.Cursor
}

// Manager is the only entry point to the audit read side (activity feed, audit screen, exports).
type Manager interface {
	Query(ctx context.Context, f Filter, page Page) (QueryResult, error)
	Export(f Filter, format FileFormat) (ExportStream, error)
	Describe(a Action) (ActionMeta, bool)
	Catalog() *Catalog
}

// DefaultManager reads through an EntryReader and stamps categories from the catalog.
type DefaultManager struct {
	reader    CategoryReader
	catalog   *Catalog
	exporters *ExporterRegistry
}

// NewManager builds the manager with the built-in export formats.
func NewManager(reader EntryReader, catalog *Catalog) *DefaultManager {
	return &DefaultManager{reader: CategoryReader{Reader: reader, Catalog: catalog}, catalog: catalog, exporters: NewExporterRegistry()}
}

// Query clamps the page size, reads one page and fills each entry's category.
func (m *DefaultManager) Query(ctx context.Context, f Filter, page Page) (QueryResult, error) {
	page.Limit = PageBounds{}.Clamp(page.Limit)
	items, next, err := m.reader.Query(ctx, f, page)
	if err != nil {
		return QueryResult{}, err
	}
	if items == nil {
		items = []LogEntry{}
	}
	return QueryResult{Items: items, Next: next}, nil
}

// Export prepares a streamed file of every entry matching f (newest first, at most MaxExportRows).
func (m *DefaultManager) Export(f Filter, format FileFormat) (ExportStream, error) {
	exporter, err := m.exporters.Get(format)
	if err != nil {
		return ExportStream{}, err
	}
	entries := PageIterator{Reader: m.reader, Filter: f, MaxRows: int(MaxExportRows)}
	return ExportStream{Exporter: exporter, Entries: entries, Format: format}, nil
}

// CategoryReader is an EntryReader that fills each entry's category from the catalog.
type CategoryReader struct {
	Reader  EntryReader
	Catalog *Catalog
}

// Query implements EntryReader.
func (r CategoryReader) Query(ctx context.Context, f Filter, page Page) ([]LogEntry, *store.Cursor, error) {
	items, next, err := r.Reader.Query(ctx, f, page)
	for i := range items {
		meta, _ := r.Catalog.Describe(items[i].Action)
		items[i].Category = meta.Category
	}
	return items, next, err
}

// Describe returns the catalog metadata of an action.
func (m *DefaultManager) Describe(a Action) (ActionMeta, bool) {
	return m.catalog.Describe(a)
}

// Catalog exposes the action catalog (recorder hook, filter parsing).
func (m *DefaultManager) Catalog() *Catalog {
	return m.catalog
}

// PageBounds applies the store's page limits (default 50, max 200).
type PageBounds struct{}

// Clamp returns a limit within (0, MaxPageLimit].
func (PageBounds) Clamp(limit int) int {
	if limit <= 0 {
		return int(store.DefaultPageLimit)
	}
	return min(limit, int(store.MaxPageLimit))
}
