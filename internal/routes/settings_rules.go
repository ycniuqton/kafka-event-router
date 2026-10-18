package routes

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
)

// RuleCode names validation codes of route settings.
type RuleCode string

const (
	CodeRequired RuleCode = "required"
	CodeTooLong  RuleCode = "too_long"
	CodeRange    RuleCode = "range"
	CodeInvalid  RuleCode = "invalid"
)

// SettingsRules validates names, tags and replicas of create/update requests.
type SettingsRules struct{}

// Check validates the given change; nil fields are skipped.
func (r SettingsRules) Check(c RouteChange) error {
	v := api.NewValidationError()
	if c.Name != nil {
		r.name(v, *c.Name)
	}
	if c.Tags != nil {
		r.tags(v, *c.Tags)
	}
	if c.Replicas != nil && (*c.Replicas < 1 || *c.Replicas > int(MaxReplicas)) {
		v.Add(string(PathReplicas), string(CodeRange), fmt.Sprintf("Replicas must be between 1 and %d.", MaxReplicas))
	}
	if c.Draft != nil && !(DraftEditor{}).IsObject(c.Draft) {
		v.Add(string(PathDraft), string(CodeInvalid), "The draft must be a JSON object route document.")
	}
	return v.OrNil()
}

func (SettingsRules) name(v *api.ValidationError, name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		v.Add(string(PathName), string(CodeRequired), "Give the route a name.")
		return
	}
	if len([]rune(trimmed)) > int(MaxNameLength) {
		v.Add(string(PathName), string(CodeTooLong), fmt.Sprintf("Names are at most %d characters.", MaxNameLength))
	}
}

func (SettingsRules) tags(v *api.ValidationError, tags []string) {
	for i, t := range tags {
		if strings.TrimSpace(t) == "" || len([]rune(t)) > int(MaxTagLength) {
			v.Add(fmt.Sprintf("%s/%d", PathTags, i), string(CodeInvalid), fmt.Sprintf("Tags are 1 to %d characters.", MaxTagLength))
		}
	}
}
