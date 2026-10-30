// Package ctxhelpers implements the helpers shared by recipe steps and the JS `ctx`
// object (mask, hash, now, formatDate, lookup, log) so both behave identically.
// It imports nothing from pkg/engine; js-sandbox binds this Set as host functions.
package ctxhelpers

// Set is the helper bundle handed to steps and to the sandbox host functions.
type Set struct {
	Mask   Masker
	Hash   Hasher
	Clock  Clock
	Date   DateFormatter
	Lookup LookupStore
	Logs   LogSinkFactory
	// Sources serves lookup steps in source mode (pkg/lookup); default NoSourceGateway.
	Sources SourceGateway
}

// DefaultSet returns helpers with system time, empty salt and empty lookups (dev and tools).
func DefaultSet() Set {
	return Set{
		Mask:    NewStyleMasker(DefaultMaskPolicy()),
		Hash:    NewSaltedHasher(StaticSaltProvider{}, ""),
		Clock:   SystemClock{},
		Date:    LayoutDateFormatter{},
		Lookup:  EmptyLookupStore{},
		Logs:    CappedLogSinkFactory{MaxBytes: int(LogLimitDefaultBytes)},
		Sources: NoSourceGateway{},
	}
}

// WithDefaults fills every nil helper from DefaultSet.
func (s Set) WithDefaults() Set {
	d := DefaultSet()
	if s.Mask == nil {
		s.Mask = d.Mask
	}
	if s.Hash == nil {
		s.Hash = d.Hash
	}
	if s.Clock == nil {
		s.Clock = d.Clock
	}
	if s.Date == nil {
		s.Date = d.Date
	}
	if s.Lookup == nil {
		s.Lookup = d.Lookup
	}
	if s.Logs == nil {
		s.Logs = d.Logs
	}
	if s.Sources == nil {
		s.Sources = d.Sources
	}
	return s
}
