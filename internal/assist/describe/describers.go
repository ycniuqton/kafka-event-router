package describe

import (
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// StepDescriber renders one step kind as a sentence.
type StepDescriber interface {
	Kind() config.StepKind
	Describe(s config.Step) string
}

// DescriberManager picks the describer by step kind (registry, no switch on kinds).
type DescriberManager struct {
	describers map[config.StepKind]StepDescriber
}

// NewDescriberManager registers the given describers.
func NewDescriberManager(ds ...StepDescriber) *DescriberManager {
	m := &DescriberManager{describers: map[config.StepKind]StepDescriber{}}
	for _, d := range ds {
		m.describers[d.Kind()] = d
	}
	return m
}

// DefaultDescriberManager covers every step kind of the route schema.
func DefaultDescriberManager() *DescriberManager {
	return NewDescriberManager(
		relocate{kind: config.StepKindRename, phrase: PhraseRename}, relocate{kind: config.StepKindCopy, phrase: PhraseCopy},
		relocate{kind: config.StepKindMove, phrase: PhraseMove}, fieldList{kind: config.StepKindRemove, phrase: PhraseRemove},
		fieldList{kind: config.StepKindKeepOnly, phrase: PhraseKeepOnly}, setValue{}, defaultValue{}, setKey{}, mask{}, hash{},
		convert{}, fixed{kind: config.StepKindDropIf, phrase: PhraseDropIf}, fixed{kind: config.StepKindJS, phrase: PhraseJS}, library{}, lookup{}, mapper{},
	)
}

// Supports reports whether kind has a describer.
func (m *DescriberManager) Supports(kind config.StepKind) bool {
	_, ok := m.describers[kind]
	return ok
}

// Describe renders s, marking disabled steps.
func (m *DescriberManager) Describe(s config.Step) string {
	d, ok := m.describers[s.Kind]
	if !ok {
		return fmt.Sprintf(string(PhraseUnknown), s.Kind)
	}
	text := d.Describe(s)
	if s.Enabled != nil && !*s.Enabled {
		return fmt.Sprintf(string(PhraseDisabled), text)
	}
	return text
}

type relocate struct {
	kind   config.StepKind
	phrase Phrase
}

func (r relocate) Kind() config.StepKind { return r.kind }
func (r relocate) Describe(s config.Step) string {
	return fmt.Sprintf(string(r.phrase), Words{}.Field(s.From), Words{}.Field(s.To))
}

type fieldList struct {
	kind   config.StepKind
	phrase Phrase
}

func (f fieldList) Kind() config.StepKind { return f.kind }
func (f fieldList) Describe(s config.Step) string {
	return fmt.Sprintf(string(f.phrase), Words{}.Fields(s.Fields))
}

type setValue struct{}

func (setValue) Kind() config.StepKind { return config.StepKindSet }
func (setValue) Describe(s config.Step) string {
	return fmt.Sprintf(string(PhraseSet), Words{}.Field(s.Field), Words{}.Source(s.Source))
}

type defaultValue struct{}

func (defaultValue) Kind() config.StepKind { return config.StepKindDefaultIfMissing }
func (defaultValue) Describe(s config.Step) string {
	return fmt.Sprintf(string(PhraseDefault), Words{}.Field(s.Field), Words{}.Value(s.Value))
}

type setKey struct{}

func (setKey) Kind() config.StepKind { return config.StepKindSetKey }
func (setKey) Describe(s config.Step) string {
	return fmt.Sprintf(string(PhraseSetKey), Words{}.Source(s.Source))
}

type mask struct{}

func (mask) Kind() config.StepKind { return config.StepKindMask }
func (mask) Describe(s config.Step) string {
	style := string(PhraseUnknownValue)
	if s.Style != nil {
		style = MaskStyleLabel[*s.Style]
	}
	return fmt.Sprintf(string(PhraseMask), Words{}.Field(s.Field), style)
}

type hash struct{}

func (hash) Kind() config.StepKind { return config.StepKindHash }
func (hash) Describe(s config.Step) string {
	return fmt.Sprintf(string(PhraseHash), Words{}.Field(s.Field))
}

type convert struct{}

func (convert) Kind() config.StepKind { return config.StepKindConvertType }
func (convert) Describe(s config.Step) string {
	target := string(PhraseUnknownValue)
	if s.TargetType != nil {
		target = string(*s.TargetType)
	}
	return fmt.Sprintf(string(PhraseConvert), Words{}.Field(s.Field), target)
}

type fixed struct {
	kind   config.StepKind
	phrase Phrase
}

func (f fixed) Kind() config.StepKind       { return f.kind }
func (f fixed) Describe(config.Step) string { return string(f.phrase) }

type library struct{}

func (library) Kind() config.StepKind { return config.StepKindLibrary }
func (library) Describe(s config.Step) string {
	version := 0
	if s.Version != nil {
		version = *s.Version
	}
	return fmt.Sprintf(string(PhraseLibrary), Words{}.Field(s.Ref), version)
}

type lookup struct{}

func (lookup) Kind() config.StepKind { return config.StepKindLookup }
func (lookup) Describe(s config.Step) string {
	target := s.Field
	if s.To != nil {
		target = s.To
	}
	if s.Ref == nil {
		return fmt.Sprintf(string(PhraseLookupRows), Words{}.Field(s.Field), len(s.Rows), Words{}.Field(target))
	}
	version := 0
	if s.Version != nil {
		version = *s.Version
	}
	return fmt.Sprintf(string(PhraseLookupTable), Words{}.Field(s.Field), *s.Ref, version, Words{}.Field(target))
}

type mapper struct{}

func (mapper) Kind() config.StepKind { return config.StepKindMap }
func (mapper) Describe(s config.Step) string {
	fields := 0
	if s.Target != nil {
		fields = len(s.Target.Fields)
	}
	return fmt.Sprintf(string(PhraseMap), fields)
}
