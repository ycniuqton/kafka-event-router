package config

import "context"

// ValidationStage selects which profile of validators runs.
type ValidationStage string

const (
	StageDraft   ValidationStage = "draft"
	StageDeploy  ValidationStage = "deploy"
	StageOffline ValidationStage = "offline"
)

// ValidatorName is the registry key of a validator rule family.
type ValidatorName string

const (
	ValidatorSchema       ValidatorName = "schema"
	ValidatorDeliveryMode ValidatorName = "delivery_mode"
	ValidatorCompile      ValidatorName = "compile"
	ValidatorLimits       ValidatorName = "limits"
	ValidatorRetryPolicy  ValidatorName = "retry_policy"
)

// ValidationContext carries the parsed document and request scope through one validation run.
type ValidationContext struct {
	Stage  ValidationStage
	TeamID string
	Doc    *Document
	halted bool
}

// Halt stops the remaining validators of the run (used when the schema is invalid).
func (vc *ValidationContext) Halt() { vc.halted = true }

// Halted reports whether a validator stopped the run.
func (vc *ValidationContext) Halted() bool { return vc.halted }

// Validator checks a route document against one rule family.
type Validator interface {
	Name() ValidatorName
	Validate(ctx context.Context, vc *ValidationContext) []Issue
}
