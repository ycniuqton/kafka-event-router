package failures

import "github.com/eventrouter/router/internal/runtime/domain"

// DispositionTable is the fixed category → disposition table of design §2.5.
type DispositionTable struct{}

// dispositions is the table body.
var dispositions = map[domain.ErrorCategory]domain.Disposition{
	domain.CategoryDecode:               domain.DispositionApplyPolicy,
	domain.CategoryStep:                 domain.DispositionApplyPolicy,
	domain.CategoryStepTimeout:          domain.DispositionRetryInMemory,
	domain.CategoryOutputInvalid:        domain.DispositionApplyPolicy,
	domain.CategoryDestinationTransient: domain.DispositionBackoffRetry,
	domain.CategorySourceTransient:      domain.DispositionBackoffRetry,
	domain.CategoryDestinationPermanent: domain.DispositionHaltRoute,
	domain.CategorySourceFatal:          domain.DispositionHaltRoute,
	domain.CategoryCompile:              domain.DispositionHaltRoute,
}

// For returns the disposition of a category; unknown categories halt (never silently lose data).
func (DispositionTable) For(c domain.ErrorCategory) domain.Disposition {
	if d, ok := dispositions[c]; ok {
		return d
	}
	return domain.DispositionHaltRoute
}

// haltReasons maps halting categories to the halt reason reported to the control plane.
var haltReasons = map[domain.ErrorCategory]domain.HaltReason{
	domain.CategorySourceFatal:          domain.HaltSourceFatal,
	domain.CategoryDestinationPermanent: domain.HaltDestinationPermanent,
}

// HaltReasonOf returns the halt reason of a halting category.
func (DispositionTable) HaltReasonOf(c domain.ErrorCategory) domain.HaltReason {
	if r, ok := haltReasons[c]; ok {
		return r
	}
	return domain.HaltDestinationPermanent
}
