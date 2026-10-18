// Command gen-tsschema prepares a JSON Schema for json-schema-to-typescript by removing
// conditional (if/then) constraints, which that generator cannot type; it prints to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Keyword names JSON Schema keywords the preparer rewrites.
type Keyword string

const (
	KeywordAllOf Keyword = "allOf"
	KeywordIf    Keyword = "if"
	KeywordThen  Keyword = "then"
	KeywordElse  Keyword = "else"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-tsschema <schema.json>")
		os.Exit(2)
	}
	if err := (Preparer{}).Run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Preparer strips conditionals recursively.
type Preparer struct{}

// Run reads the schema file and writes the prepared schema to stdout.
func (p Preparer) Run(file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(p.strip(doc))
}

func (p Preparer) strip(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return p.object(t)
	case []any:
		for i, e := range t {
			t[i] = p.strip(e)
		}
	}
	return v
}

func (p Preparer) object(m map[string]any) map[string]any {
	delete(m, string(KeywordIf))
	delete(m, string(KeywordThen))
	delete(m, string(KeywordElse))
	if all, ok := m[string(KeywordAllOf)].([]any); ok {
		kept := []any{}
		for _, e := range all {
			if s, ok := e.(map[string]any); !ok || s[string(KeywordIf)] == nil {
				kept = append(kept, e)
			}
		}
		m[string(KeywordAllOf)] = kept
		if len(kept) == 0 {
			delete(m, string(KeywordAllOf))
		}
	}
	for k, e := range m {
		m[k] = p.strip(e)
	}
	return m
}
