package importers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/library"
)

// JSONShape names the accepted top-level JSON shapes by their opening delimiter.
type JSONShape json.Delim

const (
	ShapeRows JSONShape = '['
	ShapeMap  JSONShape = '{'
)

// JSONRowImporter reads `[{"key": …, "value": …}]` or `{"<key>": <value>}`; "lines" are 1-based row numbers.
type JSONRowImporter struct{}

// Format implements library.RowImporter.
func (JSONRowImporter) Format() library.ImportFormat { return library.ImportJSON }

// Import implements library.RowImporter.
func (i JSONRowImporter) Import(r io.Reader, keyType library.ValueType, limits library.Limits) ([]library.LookupRow, []library.ImportIssue, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	c := &Collector{Book: NewKeyBook(keyType)}
	if !utf8.Valid(body) {
		c.Issue(0, string(MsgEncoding))
		return nil, c.Issues, nil
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(body, utf8BOM)))
	dec.UseNumber()
	tok, err := dec.Token()
	delim, _ := tok.(json.Delim)
	readers := map[JSONShape]func(*json.Decoder, *Collector, library.Limits) error{ShapeRows: i.rows, ShapeMap: i.entries}
	read, ok := readers[JSONShape(delim)]
	if err != nil || !ok {
		c.Issue(0, string(MsgJSONShape))
		return nil, c.Issues, nil
	}
	if err := read(dec, c, limits); err != nil {
		return nil, nil, err
	}
	return c.Rows, c.Issues, nil
}

// rows reads array elements {key, value}; a malformed body is one shape issue.
func (JSONRowImporter) rows(dec *json.Decoder, c *Collector, limits library.Limits) error {
	for line := 1; dec.More(); line++ {
		var row struct {
			Key   any `json:"key"`
			Value any `json:"value"`
		}
		if err := dec.Decode(&row); err != nil {
			c.Issue(line, string(MsgJSONRow))
			return nil
		}
		if err := c.Add(KeyText{}.Of(row.Key), row.Value, line); err != nil {
			return err
		}
		if len(c.Rows) > limits.MaxLookupRows {
			return library.ErrImportTooLarge
		}
	}
	return nil
}

// entries reads object members in document order.
func (JSONRowImporter) entries(dec *json.Decoder, c *Collector, limits library.Limits) error {
	for line := 1; dec.More(); line++ {
		tok, err := dec.Token()
		key, ok := tok.(string)
		var value any
		if err != nil || !ok || dec.Decode(&value) != nil {
			c.Issue(line, string(MsgJSONShape))
			return nil
		}
		if err := c.Add(key, value, line); err != nil {
			return err
		}
		if len(c.Rows) > limits.MaxLookupRows {
			return library.ErrImportTooLarge
		}
	}
	return nil
}

// KeyText renders a JSON key value (string, number or boolean) as the stored key text.
type KeyText struct{}

// Of returns "" for null, objects and arrays (reported as a missing key).
func (KeyText) Of(v any) string {
	switch k := v.(type) {
	case string:
		return k
	case json.Number, bool:
		return fmt.Sprint(k)
	default:
		return ""
	}
}
