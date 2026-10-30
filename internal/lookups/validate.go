package lookups

import (
	"regexp"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/lookup"
)

// Common validation texts.
const (
	MsgNameRequired    RuleMessage = "Enter a name."
	MsgNameInvalid     RuleMessage = "Use up to 100 letters, digits, spaces, dots, dashes or underscores."
	MsgDescriptionLong RuleMessage = "Keep the description under 1000 characters."
	MsgEnvInvalid      RuleMessage = "Pick dev, staging or prod."
	MsgKindInvalid     RuleMessage = "Pick http, redis, postgres or static."
	MsgConcurrency     RuleMessage = "Use a concurrency limit between 1 and 256."
	MsgRate            RuleMessage = "Use a rate of 0 (off) or more calls per second."
	MsgBreaker         RuleMessage = "Use 0 (off) to 1000 failures before the circuit opens."
	MsgBreakerOpen     RuleMessage = "Keep the circuit open between 0 and 600000 ms."
)

// SourceLimits bound common members.
type SourceLimits int

const (
	MaxDescriptionLength SourceLimits = 1000
)

// NamePattern is the allowed shape of source names.
var NamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`)

// Validator checks drafts: common members, limits and the kind settings (through the KindManager).
type Validator struct {
	Kinds *KindManager
}

// Validate returns a ValidationError with JSON Pointer paths, or nil.
func (val Validator) Validate(d Draft) error {
	v := api.NewValidationError()
	val.common(d, v)
	val.limits(d.Limits, v)
	if k, ok := val.Kinds.Get(d.Kind); ok {
		k.Validate(d, v)
	} else {
		Violation{}.Add(v, PathKind, CodeInvalid, MsgKindInvalid)
	}
	return v.OrNil()
}

func (Validator) common(d Draft, v *api.ValidationError) {
	name := strings.TrimSpace(d.Name)
	switch {
	case name == "":
		Violation{}.Add(v, PathName, CodeRequired, MsgNameRequired)
	case !NamePattern.MatchString(name):
		Violation{}.Add(v, PathName, CodeInvalid, MsgNameInvalid)
	}
	if len(d.Description) > int(MaxDescriptionLength) {
		Violation{}.Add(v, PathDescription, CodeInvalid, MsgDescriptionLong)
	}
	if !d.EnvTag.Valid() {
		Violation{}.Add(v, PathEnvTag, CodeInvalid, MsgEnvInvalid)
	}
}

func (Validator) limits(l lookup.Limits, v *api.ValidationError) {
	if l.MaxConcurrency < 1 || l.MaxConcurrency > int(MaxConcurrency) {
		Violation{}.Add(v, PathMaxConcurrency, CodeRange, MsgConcurrency)
	}
	if l.RatePerSec < 0 || l.Burst < 0 {
		Violation{}.Add(v, PathRate, CodeRange, MsgRate)
	}
	if l.BreakerFailures < 0 || l.BreakerFailures > int(MaxBreakerFailures) {
		Violation{}.Add(v, PathBreaker, CodeRange, MsgBreaker)
	}
	if l.BreakerOpenMs < 0 || l.BreakerOpenMs > int(MaxBreakerOpenMs) {
		Violation{}.Add(v, PathBreakerOpen, CodeRange, MsgBreakerOpen)
	}
}

// Prepare applies kind defaults and keeps only the secrets the kind uses.
func (val Validator) Prepare(d Draft) Draft {
	k, ok := val.Kinds.Get(d.Kind)
	if !ok {
		return d
	}
	k.Defaults(&d)
	d.Secrets = d.Secrets.Only(k.SecretFields(d))
	return d
}
