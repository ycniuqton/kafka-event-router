package config

// DefaultValidationProfile is the doc-only profile used by the API (draft, deploy) and routerctl (offline).
// Draft lists failing test cases as warnings; deploy blocks on them (rule owned by routes).
func DefaultValidationProfile() ValidationProfile {
	docOnly := []ValidatorName{ValidatorSchema, ValidatorDeliveryMode, ValidatorCompile}
	return ValidationProfile{
		Stages: map[ValidationStage][]ValidatorName{
			StageDraft:   docOnly,
			StageDeploy:  docOnly,
			StageOffline: docOnly,
		},
		Severity: map[ValidationStage]map[IssueCode]Severity{
			StageDraft:  {IssueTestCaseFailed: SeverityWarning},
			StageDeploy: {IssueTestCaseFailed: SeverityError},
		},
	}
}
