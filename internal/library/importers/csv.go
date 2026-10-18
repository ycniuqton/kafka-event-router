package importers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/library"
)

// utf8BOM is the byte-order mark spreadsheet tools put in front of UTF-8 CSV files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// CSVRowImporter reads a header line with a key column; one other column is the value as text,
// several other columns become an object value keyed by their header names.
type CSVRowImporter struct{}

// Format implements library.RowImporter.
func (CSVRowImporter) Format() library.ImportFormat { return library.ImportCSV }

// Import implements library.RowImporter.
func (i CSVRowImporter) Import(r io.Reader, keyType library.ValueType, limits library.Limits) ([]library.LookupRow, []library.ImportIssue, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	c := &Collector{Book: NewKeyBook(keyType)}
	if !utf8.Valid(body) {
		c.Issue(0, string(MsgEncoding))
		return nil, c.Issues, nil
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body, utf8BOM)))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	layout, ok := NewCSVLayout(header)
	if err != nil || !ok {
		c.Issue(1, string(MsgHeader))
		return nil, c.Issues, nil
	}
	if err := i.records(reader, layout, c, limits); err != nil {
		return nil, nil, err
	}
	return c.Rows, c.Issues, nil
}

// records reads every data line into c; past the row limit it stops with ErrImportTooLarge.
func (CSVRowImporter) records(reader *csv.Reader, layout CSVLayout, c *Collector, limits library.Limits) error {
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		var perr *csv.ParseError
		if errors.As(err, &perr) {
			c.Issue(perr.StartLine, fmt.Sprintf(string(MsgParse), perr.Err))
			continue
		}
		if err != nil {
			return err
		}
		line, _ := reader.FieldPos(0)
		if err := c.Add(layout.Key(rec), layout.Value(rec), line); err != nil {
			return err
		}
		if len(c.Rows) > limits.MaxLookupRows {
			return library.ErrImportTooLarge
		}
	}
}

// CSVLayout knows which column holds the key and how the other columns form the value.
type CSVLayout struct {
	key    int
	values []int
	names  []string
}

// NewCSVLayout reads the header; false without a key column or without any value column.
func NewCSVLayout(header []string) (CSVLayout, bool) {
	l := CSVLayout{key: -1}
	for i, h := range header {
		name := strings.TrimSpace(h)
		if strings.EqualFold(name, string(ColumnKey)) && l.key < 0 {
			l.key = i
			continue
		}
		l.values, l.names = append(l.values, i), append(l.names, name)
	}
	return l, l.key >= 0 && len(l.values) > 0
}

// Key returns the trimmed key cell.
func (l CSVLayout) Key(rec []string) string {
	if l.key >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[l.key])
}

// Value returns the value cell as text (one value column) or an object of all value columns.
func (l CSVLayout) Value(rec []string) any {
	if len(l.values) == 1 {
		return l.cell(rec, l.values[0])
	}
	obj := make(map[string]string, len(l.values))
	for n, i := range l.values {
		obj[l.names[n]] = l.cell(rec, i)
	}
	return obj
}

// cell returns column i of rec ("" for short lines).
func (CSVLayout) cell(rec []string, i int) string {
	if i < len(rec) {
		return rec[i]
	}
	return ""
}
