// Package describe turns route config steps into short deterministic sentences
// (proposal diff lines, rule-based explanations of recipe chains).
package describe

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// Phrase is a sentence template of one step kind.
type Phrase string

const (
	PhraseRename       Phrase = "Rename %s → %s"
	PhraseCopy         Phrase = "Copy %s → %s"
	PhraseMove         Phrase = "Move %s → %s"
	PhraseRemove       Phrase = "Remove %s"
	PhraseKeepOnly     Phrase = "Keep only %s"
	PhraseSet          Phrase = "Set %s to %s"
	PhraseDefault      Phrase = "Set %s to %s when it is missing"
	PhraseSetKey       Phrase = "Set the key from %s"
	PhraseMask         Phrase = "Mask %s (%s)"
	PhraseHash         Phrase = "Hash %s (SHA-256)"
	PhraseConvert      Phrase = "Convert %s to %s"
	PhraseDropIf       Phrase = "Drop events that match a condition"
	PhraseJS           Phrase = "Run a JavaScript step"
	PhraseLibrary      Phrase = "Apply library transform %s v%d"
	PhraseLookupTable  Phrase = "Look up %s in table %s v%d and write it to %s"
	PhraseLookupRows   Phrase = "Look up %s in %d inline row(s) and write it to %s"
	PhraseMap          Phrase = "Reshape the event with the visual mapper (%d output field(s))"
	PhraseUnknown      Phrase = "Unknown step %q"
	PhraseDisabled     Phrase = "%s (disabled)"
	PhraseFieldValue   Phrase = "the value of %s"
	PhraseMetadata     Phrase = "the event %s"
	PhraseFormula      Phrase = "= %s"
	PhraseNothing      Phrase = "nothing"
	PhraseListJoin     Phrase = ", "
	PhraseValuePrefix  Phrase = "value."
	PhraseUnknownValue Phrase = "?"
)

// MaskStyleLabel is the human label of a mask style.
var MaskStyleLabel = map[config.MaskStyle]string{
	config.MaskStyleEmail: "email style",
	config.MaskStyleLast4: "keep last 4",
	config.MaskStyleFull:  "full",
}

// Words renders the step members a phrase needs (field labels, values, sources).
type Words struct{}

// Field shows a path without the implicit "value." root.
func (Words) Field(p *string) string {
	if p == nil {
		return string(PhraseUnknownValue)
	}
	return strings.TrimPrefix(*p, string(PhraseValuePrefix))
}

// Fields joins several field labels.
func (w Words) Fields(ps []string) string {
	labels := make([]string, 0, len(ps))
	for i := range ps {
		labels = append(labels, w.Field(&ps[i]))
	}
	return strings.Join(labels, string(PhraseListJoin))
}

// Value renders a constant as compact JSON.
func (Words) Value(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return string(PhraseUnknownValue)
	}
	return string(raw)
}

// Source renders a value source (field, constant, metadata, formula).
func (w Words) Source(src *config.ValueSource) string {
	if src == nil {
		return string(PhraseNothing)
	}
	switch src.From {
	case config.ValueSourceKindField:
		return fmt.Sprintf(string(PhraseFieldValue), w.Field(src.Path))
	case config.ValueSourceKindMetadata:
		return fmt.Sprintf(string(PhraseMetadata), w.metadata(src.Name))
	case config.ValueSourceKindFormula:
		return fmt.Sprintf(string(PhraseFormula), w.Field(src.Expr))
	default:
		return w.Value(src.Value)
	}
}

func (Words) metadata(n *config.MetadataName) string {
	if n == nil {
		return string(PhraseUnknownValue)
	}
	return string(*n)
}
