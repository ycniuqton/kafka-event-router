package dlq

import (
	"encoding/json"
	"fmt"
	"io"
)

// ExportFormat names an export file format.
type ExportFormat string

const (
	ExportJSON   ExportFormat = "json"
	ExportNDJSON ExportFormat = "ndjson"
)

// Exporter streams detail views as one file format.
type Exporter interface {
	Format() ExportFormat
	ContentType() string
	Write(w io.Writer, views []DetailView) error
}

// JSONToken holds the JSON array punctuation.
type JSONToken string

const (
	tokenOpen  JSONToken = "["
	tokenComma JSONToken = ","
	tokenClose JSONToken = "]\n"
)

// MediaType names export content types.
type MediaType string

const (
	MediaJSON   MediaType = "application/json"
	MediaNDJSON MediaType = "application/x-ndjson"
)

// LineBreak terminates one NDJSON line.
type LineBreak string

const ndjsonLineEnd LineBreak = "\n"

// JSONArrayExporter writes one JSON array, element by element.
type JSONArrayExporter struct{}

func (JSONArrayExporter) Format() ExportFormat { return ExportJSON }
func (JSONArrayExporter) ContentType() string  { return string(MediaJSON) }

// Write implements Exporter.
func (JSONArrayExporter) Write(w io.Writer, views []DetailView) error {
	if _, err := io.WriteString(w, string(tokenOpen)); err != nil {
		return err
	}
	for i, v := range views {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if i > 0 {
			raw = append([]byte(tokenComma), raw...)
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, string(tokenClose))
	return err
}

// NDJSONExporter writes one JSON object per line (FR-17, P2).
type NDJSONExporter struct{}

func (NDJSONExporter) Format() ExportFormat { return ExportNDJSON }
func (NDJSONExporter) ContentType() string  { return string(MediaNDJSON) }

// Write implements Exporter.
func (NDJSONExporter) Write(w io.Writer, views []DetailView) error {
	for _, v := range views {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(raw, ndjsonLineEnd...)); err != nil {
			return err
		}
	}
	return nil
}

// ExportManager is the registry of export formats.
type ExportManager struct {
	exporters map[ExportFormat]Exporter
}

// NewExportManager registers exporters.
func NewExportManager(es ...Exporter) *ExportManager {
	m := &ExportManager{exporters: map[ExportFormat]Exporter{}}
	for _, e := range es {
		m.exporters[e.Format()] = e
	}
	return m
}

// For returns the exporter of a format (json when empty).
func (m *ExportManager) For(f ExportFormat) (Exporter, error) {
	if f == "" {
		f = ExportJSON
	}
	e, ok := m.exporters[f]
	if !ok {
		return nil, ErrUnknownFormat.At(string(PathFormat)).Wrap(fmt.Errorf("format %q", f))
	}
	return e, nil
}

// FileName is the export file name pattern.
type FileName string

const exportFileName FileName = "dlq-%s-%s.%s"

// ExportFileTime is the timestamp layout in export file names.
type ExportFileTime string

const exportTimeLayout ExportFileTime = "20060102T150405Z"
