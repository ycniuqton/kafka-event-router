// Package catalog builds the static engine catalogue the frontend imports
// (web/src/api/generated/engineCatalog.json): operators by field type, the coercion
// table, recipe kinds and groups, mask styles, value sources and date layouts.
package catalog

import (
	"sort"
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/formula"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Version is the catalogue format version.
type Version int

const CurrentVersion Version = 3

// Catalog is the generated engine catalogue.
type Catalog struct {
	Version              Version                              `json:"version"`
	Operators            []OperatorEntry                      `json:"operators"`
	OperatorsByFieldType map[filter.FieldType][]config.OpName `json:"operatorsByFieldType"`
	Coercion             []filter.CoercionRule                `json:"coercion"`
	Recipes              []RecipeEntry                        `json:"recipes"`
	RecipeGroups         []step.Group                         `json:"recipeGroups"`
	MaskStyles           []config.MaskStyle                   `json:"maskStyles"`
	ValueSources         []config.ValueSourceKind             `json:"valueSources"`
	MetadataNames        []config.MetadataName                `json:"metadataNames"`
	DateLayouts          []ctxhelpers.DateLayout              `json:"dateLayouts"`
	FormulaFunctions     []FunctionEntry                      `json:"formulaFunctions"`
	FormulaOperators     []formula.Symbol                     `json:"formulaOperators"`
	RedactPresets        []config.RedactPreset                `json:"redactPresets"`
	StringCases          []config.StringCase                  `json:"stringCases"`
	MapStarts            []config.MapStart                    `json:"mapStarts"`
	SourceFormats        []config.FormatType                  `json:"sourceFormats"`
	Library              LibraryEntry                         `json:"library"`
}

// LibraryEntry describes how library pins appear in steps, trace ids and lookup names.
type LibraryEntry struct {
	PinnedKinds      []config.StepKind `json:"pinnedKinds"`
	VersionSeparator string            `json:"versionSeparator"`
	StepIDSeparator  string            `json:"stepIdSeparator"`
	StepIDForm       LibraryForm       `json:"stepIdForm"`
	LookupTableNames []LibraryForm     `json:"lookupTableNames"`
	MaxDepth         step.LibraryDepth `json:"maxDepth"`
}

// LibraryForm documents a name pattern built from step ids and library pins.
type LibraryForm string

const (
	FormSplicedStepID LibraryForm = "<stepId>/<ref>@<version>/<childStepId>"
	FormTableStepID   LibraryForm = "<stepId>"
	FormTablePinned   LibraryForm = "<ref>@<version>"
	FormTableRef      LibraryForm = "<ref>"
)

// Default returns the library entry of the catalogue.
func (LibraryEntry) Default() LibraryEntry {
	return LibraryEntry{
		PinnedKinds:      []config.StepKind{config.StepKindLibrary, config.StepKindLookup},
		VersionSeparator: compilectx.LibraryVersionSeparator,
		StepIDSeparator:  step.NestedIDSeparator,
		StepIDForm:       FormSplicedStepID,
		LookupTableNames: []LibraryForm{FormTableStepID, FormTablePinned, FormTableRef},
		MaxDepth:         step.LibraryMaxDepth,
	}
}

// OperatorEntry describes one cond operator.
type OperatorEntry struct {
	Op             config.OpName      `json:"op"`
	FieldTypes     []filter.FieldType `json:"fieldTypes"`
	Operand        filter.OperandKind `json:"operand"`
	HandlesMissing bool               `json:"handlesMissing"`
}

// RecipeEntry describes one step kind.
type RecipeEntry struct {
	Kind  config.StepKind `json:"kind"`
	Group step.Group      `json:"group"`
}

// FunctionEntry describes one formula function for Studio autocomplete.
type FunctionEntry struct {
	Name        formula.FuncName         `json:"name"`
	Call        string                   `json:"call"`
	Arity       formula.Arity            `json:"arity"`
	Params      []formula.Param          `json:"params"`
	Returns     formula.ValueKind        `json:"returns"`
	Category    formula.FunctionCategory `json:"category"`
	Description string                   `json:"description"`
	Example     string                   `json:"example"`
}

// SignatureText renders `name(a, b?, ...rest)` for autocomplete.
type SignatureText struct{}

// Of renders the call form of a function.
func (SignatureText) Of(name formula.FuncName, sig formula.Signature) string {
	parts := make([]string, 0, len(sig.Params))
	for _, p := range sig.Params {
		parts = append(parts, ParamText{}.Of(p))
	}
	return string(name) + "(" + strings.Join(parts, ", ") + ")"
}

// ParamText renders one parameter with optional/variadic markers.
type ParamText struct{}

// Marker spellings of parameters.
const (
	MarkerOptional = "?"
	MarkerVariadic = "..."
)

// Of renders p.
func (ParamText) Of(p formula.Param) string {
	switch {
	case p.Variadic:
		return MarkerVariadic + p.Name
	case p.Optional:
		return p.Name + MarkerOptional
	}
	return p.Name
}

// Builder assembles the catalogue from the compiler registries.
type Builder struct {
	Registries compile.Registries
}

// Build returns the catalogue with deterministic ordering.
func (b Builder) Build() Catalog {
	c := Catalog{
		Version:              CurrentVersion,
		OperatorsByFieldType: map[filter.FieldType][]config.OpName{},
		Coercion:             filter.Coercer{}.Table(),
		RecipeGroups:         []step.Group{step.GroupFields, step.GroupValues, step.GroupPrivacy, step.GroupTypes, step.GroupFlow, step.GroupStruct, step.GroupKafka, step.GroupCode, step.GroupLibrary},
		MaskStyles:           ctxhelpers.NewStyleMasker(ctxhelpers.DefaultMaskPolicy()).Styles(),
		ValueSources:         []config.ValueSourceKind{config.ValueSourceKindField, config.ValueSourceKindConstant, config.ValueSourceKindMetadata, config.ValueSourceKindFormula},
		MetadataNames:        b.metadataNames(),
		DateLayouts:          ctxhelpers.LayoutDateFormatter{}.Layouts(),
		FormulaFunctions:     b.functions(),
		FormulaOperators:     formula.DefaultOperatorSet().Symbols(),
		RedactPresets:        []config.RedactPreset{config.RedactPresetCreditCard, config.RedactPresetPhone, config.RedactPresetEmail, config.RedactPresetIban},
		StringCases:          []config.StringCase{config.StringCaseUpper, config.StringCaseLower, config.StringCaseTitle, config.StringCaseTrim},
		MapStarts:            []config.MapStart{config.MapStartCopyInput, config.MapStartEmpty},
		SourceFormats:        b.formats(),
		Library:              LibraryEntry{}.Default(),
	}
	c.Operators = b.operators(c.OperatorsByFieldType)
	for _, sc := range b.Registries.Steps.All() {
		c.Recipes = append(c.Recipes, RecipeEntry{Kind: sc.Kind(), Group: sc.Group()})
	}
	return c
}

func (b Builder) operators(byType map[filter.FieldType][]config.OpName) []OperatorEntry {
	out := []OperatorEntry{}
	for _, op := range b.Registries.Operators.All() {
		out = append(out, OperatorEntry{Op: op.Op(), FieldTypes: op.Accepts(), Operand: op.Operand(), HandlesMissing: op.HandlesMissing()})
		for _, ft := range op.Accepts() {
			byType[ft] = append(byType[ft], op.Op())
		}
	}
	return out
}

// functions lists the formula catalogue with a rendered call signature.
func (b Builder) functions() []FunctionEntry {
	out := []FunctionEntry{}
	for _, f := range b.Registries.Formulas.Functions.All() {
		sig := f.Signature()
		out = append(out, FunctionEntry{Name: f.Name(), Call: SignatureText{}.Of(f.Name(), sig), Arity: f.Arity(), Params: sig.Params,
			Returns: sig.Returns, Category: sig.Category, Description: sig.Description, Example: sig.Example})
	}
	return out
}

// formats lists registered codec formats in a stable order.
func (b Builder) formats() []config.FormatType {
	fs := b.Registries.Codecs.Formats()
	sort.Slice(fs, func(i, j int) bool { return fs[i] < fs[j] })
	return fs
}

func (Builder) metadataNames() []config.MetadataName {
	names := []config.MetadataName{config.MetadataNameTopic, config.MetadataNamePartition, config.MetadataNameOffset,
		config.MetadataNameTimestamp, config.MetadataNameRouteName, config.MetadataNameRouteVersion}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}
