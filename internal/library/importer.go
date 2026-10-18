package library

import (
	"errors"
	"fmt"
	"io"
	"mime"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
)

// RowImporter is the base of every lookup table import format: it parses a body into rows and
// per-line issues. Register implementations in a RowImporterManager.
type RowImporter interface {
	Format() ImportFormat
	Import(r io.Reader, keyType ValueType, limits Limits) ([]LookupRow, []ImportIssue, error)
}

// ImportMessage holds import texts shown to users.
type ImportMessage string

const (
	MsgImportFormat   ImportMessage = "Upload text/csv or application/json."
	MsgImportTooLarge ImportMessage = "The import is larger than 5 MB or has more than 50,000 rows. Split the table."
	MsgImportIssues   ImportMessage = "%d more issue(s) not shown."
)

// ErrImportTooLarge is returned past the row or byte limit (413).
var ErrImportTooLarge = problem.NewError(problem.KindPayloadTooLarge, string(MsgImportTooLarge))

// RowImporterManager picks a RowImporter by media type and enforces the import limits.
type RowImporterManager struct {
	importers map[ImportFormat]RowImporter
	limits    Limits
}

// NewRowImporterManager registers importers.
func NewRowImporterManager(limits Limits, importers ...RowImporter) *RowImporterManager {
	m := &RowImporterManager{importers: map[ImportFormat]RowImporter{}, limits: limits}
	for _, i := range importers {
		m.importers[i.Format()] = i
	}
	return m
}

// Formats lists the registered media types.
func (m *RowImporterManager) Formats() []ImportFormat {
	out := make([]ImportFormat, 0, len(m.importers))
	for f := range m.importers {
		out = append(out, f)
	}
	return out
}

// Import parses body of contentType; issues become one 422 with /lines/{n} paths, limits a 413.
func (m *RowImporterManager) Import(contentType string, body io.Reader, keyType ValueType) ([]LookupRow, error) {
	media, _, _ := mime.ParseMediaType(contentType)
	imp, ok := m.importers[ImportFormat(media)]
	if !ok {
		return nil, api.NewValidationError().Add(string(FieldContent), string(CodeInvalid), string(MsgImportFormat))
	}
	limited := &LimitedReader{R: body, Max: int64(m.limits.MaxLookupBytes)}
	rows, issues, err := imp.Import(limited, keyType, m.limits)
	if limited.Exceeded || len(rows) > m.limits.MaxLookupRows || errors.Is(err, ErrImportTooLarge) {
		return nil, ErrImportTooLarge
	}
	if err != nil {
		return nil, err
	}
	return rows, IssueReport{Limit: m.limits.MaxImportIssues}.Error(issues)
}

// IssueReport turns import issues into a validation error at /lines/{line}.
type IssueReport struct {
	Limit int
}

// Error returns nil without issues; past Limit the rest is summarised in one entry.
func (r IssueReport) Error(issues []ImportIssue) error {
	verr := api.NewValidationError()
	for i, issue := range issues {
		if i == r.Limit {
			verr.Add(string(FieldLines), string(CodeInvalid), fmt.Sprintf(string(MsgImportIssues), len(issues)-r.Limit))
			break
		}
		verr.Add(string(config.Pointer(FieldLines).Index(issue.Line)), string(CodeInvalid), issue.Message)
	}
	return verr.OrNil()
}

// LimitedReader reads at most Max bytes and records whether the body was longer.
type LimitedReader struct {
	R        io.Reader
	Max      int64
	read     int64
	Exceeded bool
}

// Read implements io.Reader; past Max it reports EOF and sets Exceeded.
func (l *LimitedReader) Read(p []byte) (int, error) {
	if l.read > l.Max {
		l.Exceeded = true
		return 0, io.EOF
	}
	if rest := l.Max + 1 - l.read; int64(len(p)) > rest {
		p = p[:rest]
	}
	n, err := l.R.Read(p)
	l.read += int64(n)
	if l.read > l.Max {
		l.Exceeded = true
		return n, io.EOF
	}
	return n, err
}
