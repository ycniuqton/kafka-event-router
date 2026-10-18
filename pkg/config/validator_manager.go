package config

import (
	"context"
	"fmt"
)

// ValidationProfile configures, per stage, the ordered validators and severity overrides.
type ValidationProfile struct {
	Stages   map[ValidationStage][]ValidatorName
	Severity map[ValidationStage]map[IssueCode]Severity
}

// SeverityFor returns the configured severity of code at stage, or def when not overridden.
func (p ValidationProfile) SeverityFor(stage ValidationStage, code IssueCode, def Severity) Severity {
	if s, ok := p.Severity[stage][code]; ok {
		return s
	}
	return def
}

// ValidatorRegistry holds validator instances by name.
type ValidatorRegistry struct {
	validators map[ValidatorName]Validator
}

// NewValidatorRegistry registers the given validators by their names.
func NewValidatorRegistry(vs ...Validator) *ValidatorRegistry {
	r := &ValidatorRegistry{validators: map[ValidatorName]Validator{}}
	for _, v := range vs {
		r.validators[v.Name()] = v
	}
	return r
}

// Get returns the validator registered under name.
func (r *ValidatorRegistry) Get(name ValidatorName) (Validator, bool) {
	v, ok := r.validators[name]
	return v, ok
}

// ValidatorManager runs the doc-only validators of a stage profile; business logic uses only this.
type ValidatorManager struct {
	profile ValidationProfile
	stages  map[ValidationStage][]Validator
}

// NewValidatorManager resolves every profile entry against the registry.
func NewValidatorManager(profile ValidationProfile, reg *ValidatorRegistry) (*ValidatorManager, error) {
	m := &ValidatorManager{profile: profile, stages: map[ValidationStage][]Validator{}}
	for stage, names := range profile.Stages {
		for _, name := range names {
			v, ok := reg.Get(name)
			if !ok {
				return nil, fmt.Errorf("validation profile %q: unknown validator %q", stage, name)
			}
			m.stages[stage] = append(m.stages[stage], v)
		}
	}
	return m, nil
}

// Profile returns the profile this manager was built from.
func (m *ValidatorManager) Profile() ValidationProfile { return m.profile }

// Validate parses raw and runs the stage's validators; syntax errors become one issue at the root.
func (m *ValidatorManager) Validate(ctx context.Context, stage ValidationStage, teamID string, raw []byte) (Issues, error) {
	doc, err := Parse(raw)
	if err != nil {
		return Issues{NewError(IssueSchemaInvalid, PointerRoot, "invalid JSON: %v", err)}, nil
	}
	return m.ValidateDocument(ctx, stage, teamID, doc)
}

// ValidateDocument runs the stage's validators in order on an already parsed document.
func (m *ValidatorManager) ValidateDocument(ctx context.Context, stage ValidationStage, teamID string, doc *Document) (Issues, error) {
	validators, ok := m.stages[stage]
	if !ok {
		return nil, fmt.Errorf("unknown validation stage %q", stage)
	}
	vc := &ValidationContext{Stage: stage, TeamID: teamID, Doc: doc}
	issues := Issues{}
	for _, v := range validators {
		if vc.Halted() {
			break
		}
		issues = append(issues, v.Validate(ctx, vc)...)
	}
	return m.applySeverity(stage, issues).Sort(), nil
}

// applySeverity rewrites severities per the profile's overrides for the stage.
func (m *ValidatorManager) applySeverity(stage ValidationStage, issues Issues) Issues {
	for i := range issues {
		issues[i].Severity = m.profile.SeverityFor(stage, issues[i].Code, issues[i].Severity)
	}
	return issues
}
