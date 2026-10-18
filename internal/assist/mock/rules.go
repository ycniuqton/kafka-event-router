package mock

import (
	"regexp"

	"github.com/eventrouter/router/pkg/config"
)

// fieldWord matches a field name or path as written in an instruction.
const fieldWord = `([A-Za-z_][\w]*(?:\.[\w]+|\[\d+\])*)`

// fieldList matches "a, b and c" (the names keep_only and remove accept).
const fieldList = `(` + `[\w.\[\]]+(?:\s*(?:,|\band\b)\s*[A-Za-z_][\w.\[\]]*)*)`

// valueWord matches a literal value: a quoted string or a bare token.
const valueWord = `("[^"]*"|'[^']*'|[^\s,;]+)`

var (
	renamePattern   = regexp.MustCompile(`(?i)\brename\s+(?:the\s+)?` + fieldWord + `\s+(?:to|as|→|->)\s+` + fieldWord)
	keyPattern      = regexp.MustCompile(`(?i)\bkey\s+(?:from|by|on|to)\s+(?:the\s+)?` + fieldWord)
	keyAsPattern    = regexp.MustCompile(`(?i)\buse\s+(?:the\s+)?` + fieldWord + `\s+as\s+(?:the\s+)?key\b`)
	keepOnlyPattern = regexp.MustCompile(`(?i)\b(?:only\s+keep|keep\s+only)\s+(?:the\s+)?` + fieldList)
	defaultPattern  = regexp.MustCompile(`(?i)\bdefault\s+(?:the\s+)?` + fieldWord + `\s+to\s+` + valueWord)
	setPattern      = regexp.MustCompile(`(?i)\bset\s+(?:the\s+)?` + fieldWord + `\s+to\s+` + valueWord)
	maskPattern     = regexp.MustCompile(`(?i)\b(?:hide|mask|redact)\s+(?:the\s+)?` + fieldWord)
	hashPattern     = regexp.MustCompile(`(?i)\bhash\s+(?:the\s+)?` + fieldWord)
	removePattern   = regexp.MustCompile(`(?i)\b(?:remove|drop|delete)\s+(?:the\s+)?` + fieldList)
	emailHint       = regexp.MustCompile(`(?i)mail`)
	last4Hint       = regexp.MustCompile(`(?i)card|phone|iban|account|ssn|pan|number`)
)

// StepBuilder creates config steps with pointer members filled in.
type StepBuilder struct{}

func (StepBuilder) str(s string) *string { return &s }

// Relocate builds rename/copy/move steps.
func (b StepBuilder) Relocate(kind config.StepKind, from, to string) config.Step {
	return config.Step{Kind: kind, From: b.str(from), To: b.str(to)}
}

// Field builds single-field steps (hash).
func (b StepBuilder) Field(kind config.StepKind, field string) config.Step {
	return config.Step{Kind: kind, Field: b.str(field)}
}

// Fields builds remove/keep_only steps.
func (StepBuilder) Fields(kind config.StepKind, fields []string) config.Step {
	return config.Step{Kind: kind, Fields: fields}
}

// SetKeyFrom builds a set_key step reading a field.
func (b StepBuilder) SetKeyFrom(field string) config.Step {
	return config.Step{Kind: config.StepKindSetKey, Source: &config.ValueSource{From: config.ValueSourceKindField, Path: b.str(field)}}
}

// MaskStyleGuess picks a mask style from the field name: email style for mail fields, last 4 for numbers.
type MaskStyleGuess struct{}

// Of returns the style for path.
func (MaskStyleGuess) Of(path string) config.MaskStyle {
	if emailHint.MatchString(path) {
		return config.MaskStyleEmail
	}
	if last4Hint.MatchString(path) {
		return config.MaskStyleLast4
	}
	return config.MaskStyleFull
}

type renameRule struct{}

func (renameRule) Pattern() *regexp.Regexp { return renamePattern }
func (renameRule) Build(g []string, f *FieldResolver) []config.Step {
	from := f.Resolve(g[0])
	return []config.Step{StepBuilder{}.Relocate(config.StepKindRename, from, f.Sibling(from, g[1]))}
}

type keyRule struct{}

func (keyRule) Pattern() *regexp.Regexp { return keyPattern }
func (keyRule) Build(g []string, f *FieldResolver) []config.Step {
	return []config.Step{StepBuilder{}.SetKeyFrom(f.Resolve(g[0]))}
}

type keyAsRule struct{}

func (keyAsRule) Pattern() *regexp.Regexp { return keyAsPattern }
func (keyAsRule) Build(g []string, f *FieldResolver) []config.Step {
	return []config.Step{StepBuilder{}.SetKeyFrom(f.Resolve(g[0]))}
}

type keepOnlyRule struct{}

func (keepOnlyRule) Pattern() *regexp.Regexp { return keepOnlyPattern }
func (keepOnlyRule) Build(g []string, f *FieldResolver) []config.Step {
	return []config.Step{StepBuilder{}.Fields(config.StepKindKeepOnly, ListSplitter{}.Resolve(g[0], f))}
}

type defaultRule struct{}

func (defaultRule) Pattern() *regexp.Regexp { return defaultPattern }
func (defaultRule) Build(g []string, f *FieldResolver) []config.Step {
	s := StepBuilder{}.Field(config.StepKindDefaultIfMissing, f.Resolve(g[0]))
	s.Value = ValueLiteral{}.Parse(g[1])
	return []config.Step{s}
}

type setRule struct{}

func (setRule) Pattern() *regexp.Regexp { return setPattern }
func (setRule) Build(g []string, f *FieldResolver) []config.Step {
	s := StepBuilder{}.Field(config.StepKindSet, f.Resolve(g[0]))
	s.Source = &config.ValueSource{From: config.ValueSourceKindConstant, Value: ValueLiteral{}.Parse(g[1])}
	return []config.Step{s}
}

type maskRule struct{}

func (maskRule) Pattern() *regexp.Regexp { return maskPattern }
func (maskRule) Build(g []string, f *FieldResolver) []config.Step {
	path := f.Resolve(g[0])
	s := StepBuilder{}.Field(config.StepKindMask, path)
	style := MaskStyleGuess{}.Of(path)
	s.Style = &style
	return []config.Step{s}
}

type hashRule struct{}

func (hashRule) Pattern() *regexp.Regexp { return hashPattern }
func (hashRule) Build(g []string, f *FieldResolver) []config.Step {
	return []config.Step{StepBuilder{}.Field(config.StepKindHash, f.Resolve(g[0]))}
}

type removeRule struct{}

func (removeRule) Pattern() *regexp.Regexp { return removePattern }
func (removeRule) Build(g []string, f *FieldResolver) []config.Step {
	return []config.Step{StepBuilder{}.Fields(config.StepKindRemove, ListSplitter{}.Resolve(g[0], f))}
}
