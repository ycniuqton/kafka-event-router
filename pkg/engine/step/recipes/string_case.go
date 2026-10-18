package recipes

import (
	"strings"
	"unicode"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// stringCases are the conversions of string_case.
var stringCases = map[config.StringCase]func(string) string{
	config.StringCaseUpper: strings.ToUpper,
	config.StringCaseLower: strings.ToLower,
	config.StringCaseTrim:  strings.TrimSpace,
	config.StringCaseTitle: TitleCase{}.Of,
}

// StringCase converts text to upper, lower or title case, or trims it; non-text is unchanged.
type StringCase struct{}

func (StringCase) Kind() config.StepKind { return config.StepKindStringCase }
func (StringCase) Group() step.Group     { return step.GroupTypes }

func (StringCase) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	if raw.Case == nil || stringCases[*raw.Case] == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenCase, "string_case requires a supported case")
		return nil, false
	}
	return FieldTransformBuilder{}.Build(c, raw, caser{apply: stringCases[*raw.Case]}, true)
}

type caser struct {
	apply func(string) string
}

func (m caser) Map(_ *step.StepContext, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	return m.apply(s), nil
}

// TitleCase upper-cases the first letter of every word and lower-cases the rest.
type TitleCase struct{}

// Of returns s in title case.
func (TitleCase) Of(s string) string {
	runes := []rune(strings.ToLower(s))
	start := true
	for i, r := range runes {
		if start && unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
		}
		start = unicode.IsSpace(r) || r == '-'
	}
	return string(runes)
}
