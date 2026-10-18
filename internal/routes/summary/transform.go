package summary

import "github.com/eventrouter/router/pkg/config"

// TransformIndicator mirrors routes.TransformIndicator without importing it.
type TransformIndicator string

const (
	TransformNone   TransformIndicator = "none"
	TransformVisual TransformIndicator = "visual"
	TransformJS     TransformIndicator = "js"
)

// TransformClassifier derives the list indicator: any js step → js, any step → visual.
type TransformClassifier struct{}

// Classify inspects every branch's enabled steps.
func (TransformClassifier) Classify(cfg *config.RouteConfig) TransformIndicator {
	if cfg == nil {
		return TransformNone
	}
	out := TransformNone
	for _, b := range cfg.Branches {
		if b.Transform == nil {
			continue
		}
		for _, s := range b.Transform.Steps {
			if s.Kind == config.StepKindJS {
				return TransformJS
			}
			out = TransformVisual
		}
	}
	return out
}
