package recipes

import (
	"regexp"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// RedactDefaultReplacement replaces matches when replacement is absent.
const RedactDefaultReplacement = "[REDACTED]"

// redactPresets are the built-in RE2 patterns of redact_pattern.
var redactPresets = map[config.RedactPreset]string{
	config.RedactPresetCreditCard: `\b(?:\d[ -]?){12,18}\d\b`,
	config.RedactPresetPhone:      `\+?\d[\d ().-]{6,}\d`,
	config.RedactPresetEmail:      `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
	config.RedactPresetIban:       `\b[A-Z]{2}\d{2}(?: ?[A-Z0-9]{4}){2,7}(?: ?[A-Z0-9]{1,4})?\b`,
}

// RedactPattern replaces matches of a preset or custom RE2 pattern inside text.
type RedactPattern struct{}

func (RedactPattern) Kind() config.StepKind { return config.StepKindRedactPattern }
func (RedactPattern) Group() step.Group     { return step.GroupPrivacy }

func (RedactPattern) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	src, member := "", config.TokenPattern
	if raw.Pattern != nil {
		src = *raw.Pattern
	} else if raw.Preset != nil {
		src, member = redactPresets[*raw.Preset], config.TokenPreset
	}
	re, err := regexp.Compile(src)
	if err != nil || src == "" {
		c.Fail(config.IssueRegexInvalid, member, "redact_pattern needs a valid preset or RE2 pattern: %v", err)
		return nil, false
	}
	repl := RedactDefaultReplacement
	if raw.Replacement != nil {
		repl = *raw.Replacement
	}
	return FieldTransformBuilder{}.Build(c, raw, redactor{re: re, replacement: repl}, true)
}

type redactor struct {
	re          *regexp.Regexp
	replacement string
}

func (r redactor) Map(_ *step.StepContext, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	return r.re.ReplaceAllLiteralString(s, r.replacement), nil
}
