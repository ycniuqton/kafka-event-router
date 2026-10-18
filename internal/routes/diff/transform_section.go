package diff

import (
	"encoding/json"
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// TransformSection aligns steps by index and kind; changed parameters show as "~".
type TransformSection struct{}

func (TransformSection) Section() SectionKind { return SectionTransform }

func (TransformSection) Entries(cfg *config.RouteConfig) []Entry {
	es := Entries{}
	for i, b := range cfg.Branches {
		if b.Transform == nil {
			continue
		}
		prefix := BranchLabel{}.For(cfg, b)
		for j, s := range b.Transform.Steps {
			key := fmt.Sprintf("%s%s%s%s%d%s%s", b.ID, KeySep, KeyStep, KeySep, j, KeySep, s.Kind)
			es.AddPrefixed(key, config.StepPointer(i, j), prefix, StepDescriber{}.Describe(j, s))
		}
	}
	return es
}

// StepDescriber renders a step as "step 2: rename {params}".
type StepDescriber struct{}

// Describe returns the readable step line.
func (StepDescriber) Describe(index int, s config.Step) string {
	text := fmt.Sprintf("%s %d: %s %s", LabelStep, index+1, s.Kind, Text{}.JSON(StepParams{}.Of(s)))
	if !s.IsEnabled() {
		text += " " + string(LabelDisabled)
	}
	return text
}

// StepMember names step members that are not parameters.
type StepMember string

const (
	StepMemberKind    StepMember = "kind"
	StepMemberID      StepMember = "id"
	StepMemberEnabled StepMember = "enabled"
)

// StepParams extracts a step's parameters (everything but kind, id and enabled).
type StepParams struct{}

// Of returns the parameters as a JSON object map (sorted keys when marshalled).
func (StepParams) Of(s config.Step) map[string]any {
	raw, _ := json.Marshal(s)
	params := map[string]any{}
	_ = json.Unmarshal(raw, &params)
	for _, k := range []StepMember{StepMemberKind, StepMemberID, StepMemberEnabled} {
		delete(params, string(k))
	}
	return params
}
