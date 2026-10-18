package preview

import (
	"errors"

	"github.com/eventrouter/router/pkg/engine"
)

// StatusClassifier derives branch and event statuses from the trace, outputs and errors.
type StatusClassifier struct{}

// Branch classifies one branch: error > invalid output > passed > filtered (root false) > dropped.
func (StatusClassifier) Branch(b BranchHits, outputs []OutputDTO, branchErr *ErrorDTO) EventStatus {
	if branchErr != nil {
		return StatusError
	}
	for _, o := range outputs {
		if !o.Valid {
			return StatusError
		}
	}
	switch {
	case b.Outputs > 0:
		return StatusPassed
	case b.RootRejected():
		return StatusFiltered
	case !b.Completed:
		return StatusError
	}
	return StatusDropped
}

// statusRank orders event statuses: the highest ranked branch status becomes the event status.
var statusRank = map[EventStatus]int{StatusFiltered: 0, StatusDropped: 1, StatusPassed: 2, StatusError: 3}

// Event combines branch statuses; an event-level error wins.
func (StatusClassifier) Event(branches []BranchResultDTO, eventErr *ErrorDTO) EventStatus {
	if eventErr != nil {
		return StatusError
	}
	status := StatusFiltered
	for _, b := range branches {
		if statusRank[b.Status] > statusRank[status] {
			status = b.Status
		}
	}
	return status
}

// ErrorDescriber turns engine errors into ErrorDTOs.
type ErrorDescriber struct{}

// Describe renders err, resolving the step index from the branch trace when possible.
func (ErrorDescriber) Describe(err error, hits []BranchHits) *ErrorDTO {
	var pe *engine.ProcessError
	if !errors.As(err, &pe) {
		return &ErrorDTO{ErrorType: string(engine.ErrStep), Message: err.Error()}
	}
	out := &ErrorDTO{Step: pe.StepID, Path: string(pe.Path), ErrorType: string(pe.Type), Message: ErrorDescriber{}.message(pe)}
	for _, b := range hits {
		if i, ok := b.StepIndex(pe.StepID); ok && (pe.Branch == "" || string(pe.Branch) == b.ID) {
			out.StepIndex = &i
		}
	}
	return out
}

func (ErrorDescriber) message(pe *engine.ProcessError) string {
	if pe.Cause != nil {
		return pe.Cause.Error()
	}
	return pe.Error()
}

// BranchOf returns the branch an engine error belongs to ("" when event-wide).
func (ErrorDescriber) BranchOf(err error) string {
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		return string(pe.Branch)
	}
	return ""
}
