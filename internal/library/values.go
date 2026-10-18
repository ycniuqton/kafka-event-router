package library

import (
	"strconv"
)

// ValueType is the type of a lookup table's keys (keys are stored as text and checked against it).
type ValueType string

const (
	ValueString  ValueType = "string"
	ValueNumber  ValueType = "number"
	ValueBoolean ValueType = "boolean"
)

// keyCheckers holds how each key type accepts a key's text.
var keyCheckers = map[ValueType]func(string) bool{
	ValueString: func(string) bool { return true },
	ValueNumber: func(s string) bool {
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	},
	ValueBoolean: func(s string) bool {
		_, err := strconv.ParseBool(s)
		return err == nil && (s == strconv.FormatBool(true) || s == strconv.FormatBool(false))
	},
}

// Known reports whether t is a supported key type.
func (t ValueType) Known() bool {
	_, ok := keyCheckers[t]
	return ok
}

// OrDefault returns t, or string when t is empty.
func (t ValueType) OrDefault() ValueType {
	if t == "" {
		return ValueString
	}
	return t
}

// Accepts reports whether key is a valid key of this type.
func (t ValueType) Accepts(key string) bool {
	check, ok := keyCheckers[t.OrDefault()]
	return ok && check(key)
}

// ImportFormat is the media type of a lookup table import body.
type ImportFormat string

const (
	ImportCSV  ImportFormat = "text/csv"
	ImportJSON ImportFormat = "application/json"
)

// ImportIssue is one problem of an import, by 1-based line (0 = the whole body, e.g. encoding).
type ImportIssue struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}
