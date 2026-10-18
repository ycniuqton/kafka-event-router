package ctxhelpers

// LookupStore resolves keys of compiled lookup tables (inline P2, library P3).
type LookupStore interface {
	Lookup(table, key string) (any, bool)
}

// EmptyLookupStore has no tables.
type EmptyLookupStore struct{}

func (EmptyLookupStore) Lookup(string, string) (any, bool) { return nil, false }

// LogLimit bounds buffered user logs.
type LogLimit int

const LogLimitDefaultBytes LogLimit = 4096

// LogSink buffers user log lines up to a byte cap.
type LogSink interface {
	Log(line string)
	Lines() []string
	Truncated() bool
}

// LogSinkFactory creates one sink per call.
type LogSinkFactory interface {
	New() LogSink
}

// CappedLogSinkFactory creates CappedLogSinks.
type CappedLogSinkFactory struct {
	MaxBytes int
}

func (f CappedLogSinkFactory) New() LogSink { return &CappedLogSink{max: f.MaxBytes} }

// CappedLogSink drops lines once MaxBytes is reached and flags truncation.
type CappedLogSink struct {
	max       int
	used      int
	lines     []string
	truncated bool
}

func (s *CappedLogSink) Log(line string) {
	if s.used+len(line) > s.max {
		s.truncated = true
		return
	}
	s.used += len(line)
	s.lines = append(s.lines, line)
}

func (s *CappedLogSink) Lines() []string { return s.lines }

func (s *CappedLogSink) Truncated() bool { return s.truncated }

// TableLookupStore holds the lookup tables compiled from one route version: inline rows
// by step id and pinned library tables by `<ref>@<version>` and, when unambiguous, `<ref>`.
type TableLookupStore struct {
	tables  map[string]map[string]any
	aliases map[string]string
}

// NewTableLookupStore returns an empty store.
func NewTableLookupStore() *TableLookupStore {
	return &TableLookupStore{tables: map[string]map[string]any{}, aliases: map[string]string{}}
}

// Register adds or replaces a table; it is called at compile time only.
func (s *TableLookupStore) Register(table string, rows map[string]any) {
	s.tables[table] = rows
}

// RegisterAlias names a registered table by a second, lower-priority name (a bare
// library ref); an alias claimed by two tables resolves to none.
func (s *TableLookupStore) RegisterAlias(alias, table string) {
	if prev, seen := s.aliases[alias]; seen && prev != table {
		s.aliases[alias] = ""
		return
	}
	s.aliases[alias] = table
}

// Lookup reads a table by name, then by alias.
func (s *TableLookupStore) Lookup(table, key string) (any, bool) {
	rows, ok := s.tables[table]
	if !ok {
		rows = s.tables[s.aliases[table]]
	}
	v, ok := rows[key]
	return v, ok
}

// LayeredLookupStore asks each store in order and returns the first hit.
type LayeredLookupStore struct {
	Layers []LookupStore
}

func (s LayeredLookupStore) Lookup(table, key string) (any, bool) {
	for _, l := range s.Layers {
		if v, ok := l.Lookup(table, key); ok {
			return v, true
		}
	}
	return nil, false
}
