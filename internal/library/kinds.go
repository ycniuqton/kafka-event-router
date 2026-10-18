package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/config"
)

// TableSchema names the Postgres tables and columns of one object kind; KeyType is "" when the kind has none.
type TableSchema struct {
	Objects    string
	Versions   string
	ForeignKey string
	Content    string
	KeyType    string
}

// ContentKind is the base of every library object kind: where it is stored, how a version's content
// is validated and compiled, and how routes reference it. Register implementations in a KindManager.
type ContentKind interface {
	Kind() ObjectKind
	Schema() TableSchema
	// Validate checks one version's content and returns its size (steps or rows).
	Validate(content json.RawMessage, keyType ValueType) (int, error)
	// Compile proves a valid version runs on the engine for the owner team (422 with paths when it does not).
	Compile(ctx context.Context, content json.RawMessage, team uuid.UUID) error
	// UsagePath is the jsonpath (variable $n = object name) matching references in a route document.
	UsagePath() string
}

// KindManager selects the ContentKind of an object kind.
type KindManager struct {
	kinds map[ObjectKind]ContentKind
}

// NewKindManager registers kinds.
func NewKindManager(kinds ...ContentKind) *KindManager {
	m := &KindManager{kinds: map[ObjectKind]ContentKind{}}
	for _, k := range kinds {
		m.kinds[k.Kind()] = k
	}
	return m
}

// DefaultKindManager registers transforms (compiled by compiler; nil skips the compile) and lookup tables.
func DefaultKindManager(checker *assist.StepChecker, limits Limits, compiler StepCompiler) *KindManager {
	return NewKindManager(TransformKind{checker: checker, limits: limits, compiler: compiler}, LookupTableKind{limits: limits})
}

// Get returns the kind or ErrUnknownKind.
func (m *KindManager) Get(kind ObjectKind) (ContentKind, error) {
	k, ok := m.kinds[kind]
	if !ok {
		return nil, ErrUnknownKind
	}
	return k, nil
}

// TransformKind stores reusable step lists, validated against the route schema's step definition.
type TransformKind struct {
	checker  *assist.StepChecker
	limits   Limits
	compiler StepCompiler
}

func (TransformKind) Kind() ObjectKind { return KindTransform }

func (TransformKind) Schema() TableSchema {
	return TableSchema{Objects: "transforms", Versions: "transform_versions", ForeignKey: "transform_id", Content: "steps"}
}

func (TransformKind) UsagePath() string {
	return `$.branches[*].transform.steps[*] ? (@.kind == "library" && @.ref == $n)`
}

// Validate decodes the steps and checks each one (schema + path grammar); issues carry /steps/i paths.
func (k TransformKind) Validate(content json.RawMessage, _ ValueType) (int, error) {
	var steps []config.Step
	if err := json.Unmarshal(content, &steps); err != nil {
		return 0, api.NewValidationError().Add(string(FieldSteps), string(CodeInvalid), err.Error())
	}
	verr := api.NewValidationError()
	if len(steps) == 0 {
		verr.Add(string(FieldSteps), string(CodeRequired), string(MsgStepsRequired))
	}
	if len(steps) > k.limits.MaxSteps {
		verr.Add(string(FieldSteps), string(CodeTooLarge), string(MsgTooManySteps))
	}
	for i, s := range steps {
		for _, issue := range k.checker.Check(config.Pointer(FieldSteps).Index(i), s) {
			verr.Add(string(issue.Path), string(issue.Code), issue.Message)
		}
	}
	return len(steps), verr.OrNil()
}

// Compile runs the engine compiler over the steps (library refs and JS included).
func (k TransformKind) Compile(ctx context.Context, content json.RawMessage, team uuid.UUID) error {
	if k.compiler == nil {
		return nil
	}
	return k.compiler.Compile(ctx, content, team)
}

// LookupRow is one key → value row of a lookup table.
type LookupRow struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// LookupTableKind stores versioned key → value rows.
type LookupTableKind struct {
	limits Limits
}

func (LookupTableKind) Kind() ObjectKind { return KindLookupTable }

func (LookupTableKind) Schema() TableSchema {
	return TableSchema{Objects: "lookup_tables", Versions: "lookup_table_versions", ForeignKey: "lookup_table_id", Content: "rows", KeyType: "key_type"}
}

func (LookupTableKind) UsagePath() string {
	return `$.branches[*].transform.steps[*] ? (@.kind == "lookup" && @.ref == $n)`
}

// Compile has nothing to compile: rows are data.
func (LookupTableKind) Compile(context.Context, json.RawMessage, uuid.UUID) error { return nil }

// Validate checks size limits, the row shape, unique non-empty keys and the key type.
func (k LookupTableKind) Validate(content json.RawMessage, keyType ValueType) (int, error) {
	if len(content) > k.limits.MaxLookupBytes {
		return 0, api.NewValidationError().Add(string(FieldRows), string(CodeTooLarge), string(MsgTooLarge))
	}
	var rows []LookupRow
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rows); err != nil {
		return 0, api.NewValidationError().Add(string(FieldRows), string(CodeInvalid), string(MsgRowsInvalid))
	}
	verr := api.NewValidationError()
	if len(rows) == 0 {
		verr.Add(string(FieldRows), string(CodeRequired), string(MsgRowsRequired))
	}
	if len(rows) > k.limits.MaxLookupRows {
		verr.Add(string(FieldRows), string(CodeTooLarge), string(MsgTooManyRows))
	}
	RowKeyRules{KeyType: keyType}.Check(rows, verr)
	return len(rows), verr.OrNil()
}

// RowKeyRules checks the keys of rows: non-empty, unique and of the table's key type.
type RowKeyRules struct {
	KeyType ValueType
}

// Check adds one error per bad key at /rows/i/key.
func (r RowKeyRules) Check(rows []LookupRow, verr *api.ValidationError) {
	seen := make(map[string]bool, len(rows))
	for i, row := range rows {
		path := string(config.Pointer(FieldRows).Index(i).Key(string(MemberRowKey)))
		if row.Key == "" || seen[row.Key] {
			verr.Add(path, string(CodeInvalid), string(MsgRowKey))
		} else if !r.KeyType.Accepts(row.Key) {
			verr.Add(path, string(CodeInvalid), fmt.Sprintf(string(MsgRowKeyType), r.KeyType.OrDefault()))
		}
		seen[row.Key] = true
	}
}
