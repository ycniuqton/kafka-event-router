// Package importers holds the lookup table RowImporter implementations (CSV, JSON); wiring registers
// them in library.RowImporterManager so business code never names a concrete importer.
package importers

import (
	"encoding/json"
	"fmt"

	"github.com/eventrouter/router/internal/library"
)

// Message holds per-line import texts shown to users.
type Message string

const (
	MsgEncoding     Message = "The file is not UTF-8 encoded. Save it as UTF-8 and upload again."
	MsgHeader       Message = "The first line must be a header with a \"key\" column (e.g. key,value)."
	MsgKeyMissing   Message = "The key is empty."
	MsgKeyDuplicate Message = "Duplicate key %q (first seen on line %d)."
	MsgKeyType      Message = "Key %q is not a %s."
	MsgParse        Message = "Cannot read this line: %s"
	MsgJSONShape    Message = "Use a JSON array of {\"key\", \"value\"} objects or one object of key → value."
	MsgJSONRow      Message = "Each row must be an object with a key and a value."
)

// Column names CSV header columns with a meaning.
type Column string

const ColumnKey Column = "key"

// Register adds every importer to the manager's registry (cmd/api wiring).
func Register(limits library.Limits) *library.RowImporterManager {
	return library.NewRowImporterManager(limits, CSVRowImporter{}, JSONRowImporter{})
}

// KeyBook checks keys of one import: non-empty, unique (remembering the first line) and typed.
type KeyBook struct {
	keyType library.ValueType
	seen    map[string]int
}

// NewKeyBook starts an empty book for keys of keyType.
func NewKeyBook(keyType library.ValueType) *KeyBook {
	return &KeyBook{keyType: keyType, seen: map[string]int{}}
}

// Check returns the issue of key on line, or "" when the key is fine (and records it).
func (b *KeyBook) Check(key string, line int) string {
	if key == "" {
		return string(MsgKeyMissing)
	}
	if first, ok := b.seen[key]; ok {
		return fmt.Sprintf(string(MsgKeyDuplicate), key, first)
	}
	if !b.keyType.Accepts(key) {
		return fmt.Sprintf(string(MsgKeyType), key, b.keyType.OrDefault())
	}
	b.seen[key] = line
	return ""
}

// Collector accumulates rows and issues of one import.
type Collector struct {
	Book   *KeyBook
	Rows   []library.LookupRow
	Issues []library.ImportIssue
}

// Add records a row with value on line, or an issue when its key is bad.
func (c *Collector) Add(key string, value any, line int) error {
	if msg := c.Book.Check(key, line); msg != "" {
		c.Issue(line, msg)
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.Rows = append(c.Rows, library.LookupRow{Key: key, Value: raw})
	return nil
}

// Issue records one problem.
func (c *Collector) Issue(line int, msg string) {
	c.Issues = append(c.Issues, library.ImportIssue{Line: line, Message: msg})
}
