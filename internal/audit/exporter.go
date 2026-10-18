package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// FileFormat names an audit export file format (GET /audit-log/export?format=).
type FileFormat string

const (
	FormatJSONL FileFormat = "jsonl"
	FormatCSV   FileFormat = "csv"
)

// ExportMedia names the content types of export files.
type ExportMedia string

const (
	MediaJSONL ExportMedia = "application/x-ndjson"
	MediaCSV   ExportMedia = "text/csv; charset=utf-8"
)

// EntryIterator visits audit entries in export order until visit fails or the entries run out.
type EntryIterator interface {
	Each(ctx context.Context, visit func(LogEntry) error) error
}

// Exporter is the base of the extensible export-format domain: it streams entries as one file format.
type Exporter interface {
	Kind() FileFormat
	ContentType() string
	Write(ctx context.Context, w io.Writer, it EntryIterator) error
}

// JSONLExporter writes one JSON object per line, the same shape as GET /audit-log items.
type JSONLExporter struct{}

func (JSONLExporter) Kind() FileFormat    { return FormatJSONL }
func (JSONLExporter) ContentType() string { return string(MediaJSONL) }

// Write implements Exporter.
func (JSONLExporter) Write(ctx context.Context, w io.Writer, it EntryIterator) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return it.Each(ctx, func(e LogEntry) error { return enc.Encode(e) })
}

// CSVColumn names one column of the CSV export, in file order.
type CSVColumn string

const (
	ColID          CSVColumn = "id"
	ColAt          CSVColumn = "at"
	ColActorID     CSVColumn = "actor_id"
	ColActorName   CSVColumn = "actor_name"
	ColActorKind   CSVColumn = "actor_kind"
	ColAction      CSVColumn = "action"
	ColCategory    CSVColumn = "category"
	ColObjectType  CSVColumn = "object_type"
	ColObjectID    CSVColumn = "object_id"
	ColObjectLabel CSVColumn = "object_label"
	ColTeamID      CSVColumn = "team_id"
	ColIP          CSVColumn = "ip"
	ColRequestID   CSVColumn = "request_id"
	ColSensitive   CSVColumn = "sensitive"
	ColDiff        CSVColumn = "diff"
)

// CSVColumns is the header row of the CSV export.
var CSVColumns = []CSVColumn{ColID, ColAt, ColActorID, ColActorName, ColActorKind, ColAction, ColCategory, ColObjectType,
	ColObjectID, ColObjectLabel, ColTeamID, ColIP, ColRequestID, ColSensitive, ColDiff}

// CSVExporter writes RFC 4180 CSV with a header row; cells that a spreadsheet would run as a formula are neutralised.
type CSVExporter struct{}

func (CSVExporter) Kind() FileFormat    { return FormatCSV }
func (CSVExporter) ContentType() string { return string(MediaCSV) }

// Write implements Exporter.
func (CSVExporter) Write(ctx context.Context, w io.Writer, it EntryIterator) error {
	cw := csv.NewWriter(w)
	header := make([]string, len(CSVColumns))
	for i, c := range CSVColumns {
		header[i] = string(c)
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	err := it.Each(ctx, func(e LogEntry) error { return cw.Write(CSVRow{}.Of(e)) })
	cw.Flush()
	if err != nil {
		return err
	}
	return cw.Error()
}

// CSVRow renders one entry as CSV cells in CSVColumns order.
type CSVRow struct{}

// Of returns the cells of e.
func (CSVRow) Of(e LogEntry) []string {
	var cell FormulaGuard
	actorID, teamID := "", ""
	if e.Actor.ID != nil {
		actorID = e.Actor.ID.String()
	}
	if e.TeamID != nil {
		teamID = e.TeamID.String()
	}
	return []string{strconv.FormatInt(e.ID, 10), e.At.UTC().Format(time.RFC3339Nano), actorID, cell.Safe(e.Actor.Name),
		string(e.Actor.Kind), string(e.Action), string(e.Category), string(e.ObjectType), cell.Safe(e.ObjectID),
		cell.Safe(e.ObjectLabel), teamID, e.IP, cell.Safe(e.RequestID), strconv.FormatBool(e.Sensitive), cell.Safe(string(e.Diff))}
}

// FormulaGuard prefixes cells starting with a spreadsheet formula trigger (CSV injection, OWASP).
type FormulaGuard struct{}

// FormulaTriggers are the leading characters spreadsheets evaluate.
const FormulaTriggers = "=+-@\t\r"

// FormulaEscape is prepended to a triggering cell so it stays text.
const FormulaEscape = "'"

// Safe returns s, escaped when it would start a formula.
func (FormulaGuard) Safe(s string) string {
	if s != "" && strings.ContainsRune(FormulaTriggers, rune(s[0])) {
		return FormulaEscape + s
	}
	return s
}

// ExporterRegistry maps file formats to exporters (config-free selection by request parameter).
type ExporterRegistry struct {
	exporters map[FileFormat]Exporter
}

// NewExporterRegistry returns the registry with JSONL and CSV registered.
func NewExporterRegistry() *ExporterRegistry {
	r := &ExporterRegistry{exporters: map[FileFormat]Exporter{}}
	r.Register(JSONLExporter{})
	r.Register(CSVExporter{})
	return r
}

// Register adds or replaces the exporter of its format.
func (r *ExporterRegistry) Register(e Exporter) {
	r.exporters[e.Kind()] = e
}

// Get returns the exporter of format; unknown formats are a 422 at /format.
func (r *ExporterRegistry) Get(format FileFormat) (Exporter, error) {
	e, ok := r.exporters[format]
	if !ok {
		r := &filterReader{errs: &FilterError{}}
		r.fail(ParamFormat, RuleFormat, "Use jsonl or csv.")
		return nil, r.errs
	}
	return e, nil
}

// ExportLimit bounds one export (rows) and the keyset page it reads at a time.
type ExportLimit int

const (
	MaxExportRows  ExportLimit = 1_000_000
	ExportPageSize ExportLimit = 500
)

// PageIterator walks the filtered log newest first through keyset pages of an EntryReader.
type PageIterator struct {
	Reader  EntryReader
	Filter  Filter
	MaxRows int
}

// Each implements EntryIterator; it stops after MaxRows entries.
func (p PageIterator) Each(ctx context.Context, visit func(LogEntry) error) error {
	page, seen := Page{Limit: int(ExportPageSize)}, 0
	for {
		items, next, err := p.Reader.Query(ctx, p.Filter, page)
		if err != nil {
			return err
		}
		for _, e := range items {
			if seen >= p.MaxRows {
				return nil
			}
			if err := visit(e); err != nil {
				return err
			}
			seen++
		}
		if next == nil {
			return nil
		}
		page.After = next
	}
}

// ExportStream is a prepared export: the chosen format over the filtered entries.
type ExportStream struct {
	Exporter Exporter
	Entries  EntryIterator
	Format   FileFormat
}

// FileNamePattern names downloaded export files (audit-log-<UTC timestamp>.<format>).
const FileNamePattern = "audit-log-%s.%s"

// FileStamp is the timestamp layout inside export file names.
const FileStamp = "20060102-150405"

// FileName returns the download name of the export at now.
func (s ExportStream) FileName(now time.Time) string {
	return fmt.Sprintf(FileNamePattern, now.UTC().Format(FileStamp), s.Format)
}

// WriteTo streams the file to w.
func (s ExportStream) WriteTo(ctx context.Context, w io.Writer) error {
	return s.Exporter.Write(ctx, w, s.Entries)
}
