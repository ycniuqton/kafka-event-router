package preview

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// BranchLayout maps branch ids to their index in the route document.
type BranchLayout map[string]int

// NewBranchLayout indexes the branches of cfg.
func NewBranchLayout(cfg *config.RouteConfig) BranchLayout {
	l := BranchLayout{}
	for i, b := range cfg.Branches {
		l[b.ID] = i
	}
	return l
}

// FilterBase returns the absolute pointer of a branch's filter.
func (l BranchLayout) FilterBase(branch string) config.Pointer {
	return config.BranchPointer(l[branch]).Token(config.TokenFilter)
}

// TraceEncoder turns one event's trace, outputs and error into branch DTOs.
type TraceEncoder struct {
	Layout     BranchLayout
	Classifier StatusClassifier
}

// Encode builds the branch results; branchErr applies to the branch named by the engine error.
func (e TraceEncoder) Encode(hits []BranchHits, outputs map[string][]OutputDTO, procErr error) []BranchResultDTO {
	errBranch := ErrorDescriber{}.BranchOf(procErr)
	out := make([]BranchResultDTO, 0, len(hits))
	for _, b := range hits {
		var bErr *ErrorDTO
		if procErr != nil && (errBranch == "" || errBranch == b.ID) {
			bErr = ErrorDescriber{}.Describe(procErr, []BranchHits{b})
		}
		outs := outputs[b.ID]
		if outs == nil {
			outs = []OutputDTO{}
		}
		out = append(out, BranchResultDTO{
			Branch: b.ID, Status: e.Classifier.Branch(b, outs, bErr), Filter: e.filter(b), Steps: e.steps(b), Error: bErr, Outputs: outs,
		})
	}
	return out
}

func (e TraceEncoder) filter(b BranchHits) []FilterNodeDTO {
	base := e.Layout.FilterBase(b.ID)
	out := make([]FilterNodeDTO, 0, len(b.Filter))
	for _, n := range b.Filter {
		out = append(out, FilterNodeDTO{Path: string(base.Join(config.Pointer(n.Node))), Result: n.Result, Warning: n.Warning})
	}
	return out
}

func (e TraceEncoder) steps(b BranchHits) []StepTraceDTO {
	out := make([]StepTraceDTO, 0, len(b.Steps))
	for i, s := range b.Steps {
		dto := StepTraceDTO{Index: i, ID: s.ID, Kind: s.Kind, Patch: s.Patch, DurationUs: s.Duration.Microseconds(), Logs: s.Logs}
		if dto.Patch == nil {
			dto.Patch = []trace.PatchOp{}
		}
		if s.Err != nil {
			msg := s.Err.Error()
			dto.Error = &msg
		}
		out = append(out, dto)
	}
	return out
}

// WarningKey separates path and message in the collector's dedup key.
type WarningKey string

const warningKeySeparator WarningKey = "\x00"

// WarningCollector gathers distinct coercion warnings of a run as issues.
type WarningCollector struct {
	seen   map[string]bool
	issues []IssueDTO
}

// NewWarningCollector returns an empty collector.
func NewWarningCollector() *WarningCollector {
	return &WarningCollector{seen: map[string]bool{}, issues: []IssueDTO{}}
}

// Add records the node warnings of branch results once per path and message.
func (c *WarningCollector) Add(branches []BranchResultDTO) {
	for _, b := range branches {
		for _, n := range b.Filter {
			key := n.Path + string(warningKeySeparator) + n.Warning
			if n.Warning == "" || c.seen[key] {
				continue
			}
			c.seen[key] = true
			c.issues = append(c.issues, IssueDTO{Path: n.Path, Severity: string(config.SeverityWarning), Code: n.Warning, Message: n.Warning})
		}
	}
}

// Issues returns the collected warnings.
func (c *WarningCollector) Issues() []IssueDTO { return c.issues }
