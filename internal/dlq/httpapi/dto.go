package httpapi

import (
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/pkg/config"
)

// ResultDTO is one per-event result.
type ResultDTO struct {
	ID          string               `json:"id"`
	Outcome     dlq.Outcome          `json:"outcome"`
	Destination *dlq.Destination     `json:"destination,omitempty"`
	ErrorType   dlq.RetryFailureKind `json:"errorType,omitempty"`
	Message     string               `json:"message,omitempty"`
	Reason      dlq.SkipReason       `json:"reason,omitempty"`
}

// RetryDTO answers POST …/retry (design §4.4).
type RetryDTO struct {
	ActiveVersionNo int                 `json:"activeVersionNo"`
	Results         []ResultDTO         `json:"results"`
	Counts          map[dlq.Outcome]int `json:"counts"`
	Remaining       int                 `json:"remaining"`
	Warnings        []dlq.Warning       `json:"warnings,omitempty"`
	DeliveryMode    config.DeliveryMode `json:"deliveryMode,omitempty"`
	AtLeastOnce     bool                `json:"deliveredAtLeastOnce"`
}

// DiscardDTO answers POST …/discard.
type DiscardDTO struct {
	Discarded int           `json:"discarded"`
	Skipped   []ResultDTO   `json:"skipped"`
	Failed    []ResultDTO   `json:"failed,omitempty"`
	Remaining int           `json:"remaining"`
	Warnings  []dlq.Warning `json:"warnings,omitempty"`
}

// RestoreDTO answers POST …/restore.
type RestoreDTO struct {
	Restored  int           `json:"restored"`
	Skipped   []ResultDTO   `json:"skipped"`
	Failed    []ResultDTO   `json:"failed,omitempty"`
	Remaining int           `json:"remaining"`
	Warnings  []dlq.Warning `json:"warnings,omitempty"`
}

// ReportDTOs renders action reports.
type ReportDTOs struct{}

// Retry renders a retry report.
func (ReportDTOs) Retry(r dlq.ActionReport) RetryDTO {
	counts := map[dlq.Outcome]int{dlq.OutcomeSent: 0, dlq.OutcomeFiltered: 0, dlq.OutcomeFailed: 0, dlq.OutcomeSkipped: 0}
	out := RetryDTO{ActiveVersionNo: r.ActiveVersionNo, Results: []ResultDTO{}, Counts: counts, Remaining: r.Remaining, Warnings: r.Warnings,
		DeliveryMode: r.Delivery.Mode, AtLeastOnce: r.Delivery.DeliveredAtLeastOnce}
	for _, res := range r.Results {
		out.Results = append(out.Results, ReportDTOs{}.Result(res))
		counts[res.Outcome]++
	}
	return out
}

// Discard renders a discard report.
func (ReportDTOs) Discard(r dlq.ActionReport) DiscardDTO {
	out := DiscardDTO{Skipped: []ResultDTO{}, Remaining: r.Remaining, Warnings: r.Warnings}
	for _, res := range r.Results {
		switch res.Outcome {
		case dlq.OutcomeDiscarded:
			out.Discarded++
		case dlq.OutcomeSkipped:
			out.Skipped = append(out.Skipped, ReportDTOs{}.Result(res))
		default:
			out.Failed = append(out.Failed, ReportDTOs{}.Result(res))
		}
	}
	return out
}

// Result renders one result.
func (ReportDTOs) Result(res dlq.ItemResult) ResultDTO {
	return ResultDTO{ID: dlq.RefCodec{}.Encode(res.Ref), Outcome: res.Outcome, Destination: res.Destination, ErrorType: res.ErrorType, Message: res.Message, Reason: res.Reason}
}

// Restore renders a restore report.
func (ReportDTOs) Restore(r dlq.ActionReport) RestoreDTO {
	out := RestoreDTO{Skipped: []ResultDTO{}, Remaining: r.Remaining, Warnings: r.Warnings}
	for _, res := range r.Results {
		switch res.Outcome {
		case dlq.OutcomeRestored:
			out.Restored++
		case dlq.OutcomeSkipped:
			out.Skipped = append(out.Skipped, ReportDTOs{}.Result(res))
		default:
			out.Failed = append(out.Failed, ReportDTOs{}.Result(res))
		}
	}
	return out
}
