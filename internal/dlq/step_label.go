package dlq

import (
	"strconv"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// StepLabelPart is a fixed fragment of a human step label ("Step 2 · JavaScript").
type StepLabelPart string

const (
	StepLabelPrefix    StepLabelPart = "Step "
	StepLabelSeparator StepLabelPart = " · "
	StepLabelFilter    StepLabelPart = "Filter"
)

// StepKindNames names step kinds the way the builder's step menu does.
type StepKindNames struct{}

// Of returns the display name of kind (the raw kind when unknown).
func (StepKindNames) Of(kind config.StepKind) string {
	names := map[config.StepKind]string{
		config.StepKindJS: "JavaScript", config.StepKindRename: "Rename field", config.StepKindMask: "Mask",
		config.StepKindHash: "Hash (SHA-256)", config.StepKindRemove: "Remove field", config.StepKindSet: "Set value",
		config.StepKindSetKey: "Set key from field", config.StepKindCopy: "Copy field", config.StepKindMove: "Move field",
		config.StepKindKeepOnly: "Keep only", config.StepKindDropIf: "Drop if", config.StepKindLibrary: "Library step",
		config.StepKindConvertType: "Convert type", config.StepKindDefaultIfMissing: "Default if missing",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return string(kind)
}

// StepKey addresses one step of one branch of one version.
type StepKey struct {
	VersionNo int
	Branch    string
	Step      string
}

// StepLabels maps the step ids written into DLQ records to human labels.
type StepLabels map[StepKey]string

// Of returns the label of a failing step; unknown steps keep their id.
func (l StepLabels) Of(versionNo int, branch, stepID string) string {
	if label, ok := l[StepKey{VersionNo: versionNo, Branch: branch, Step: stepID}]; ok {
		return label
	}
	if stepID == engine.StepFilter {
		return string(StepLabelFilter)
	}
	return stepID
}

// StepLabelsOf derives step labels from stored version configs.
type StepLabelsOf struct{}

// Versions labels every step of every branch of versions; unparsable versions are skipped.
func (StepLabelsOf) Versions(versions []VersionConfig) StepLabels {
	out := StepLabels{}
	for _, v := range versions {
		cfg, ok := (StepLabelsOf{}).config(v)
		if !ok {
			continue
		}
		for _, b := range cfg.Branches {
			(StepLabelsOf{}).branch(out, v.No, b)
		}
	}
	return out
}

func (StepLabelsOf) config(v VersionConfig) (*config.RouteConfig, bool) {
	doc, err := config.Parse(v.Config)
	if err != nil {
		return nil, false
	}
	cfg, err := doc.Config()
	return cfg, err == nil
}

func (StepLabelsOf) branch(out StepLabels, versionNo int, b config.Branch) {
	out[StepKey{VersionNo: versionNo, Branch: b.ID, Step: engine.StepFilter}] = string(StepLabelFilter)
	if b.Transform == nil {
		return
	}
	for i, s := range b.Transform.Steps {
		label := string(StepLabelPrefix) + strconv.Itoa(i+1) + string(StepLabelSeparator) + (StepKindNames{}).Of(s.Kind)
		out[StepKey{VersionNo: versionNo, Branch: b.ID, Step: step.IDFor(s, i)}] = label
	}
}
