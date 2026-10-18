package preview

import (
	"time"

	"github.com/eventrouter/router/pkg/engine/trace"
)

// NodeHit is one evaluated filter node (pointer relative to the branch filter).
type NodeHit struct {
	Node    string
	Result  bool
	Warning string
}

// StepHit is one executed step with its patch against the previous snapshot.
type StepHit struct {
	ID       string
	Kind     string
	Patch    []trace.PatchOp
	Duration time.Duration
	Logs     []string
	Err      error
}

// BranchHits collects the diagnostics of one branch for one event.
type BranchHits struct {
	ID        string
	Filter    []NodeHit
	Steps     []StepHit
	Outputs   int
	Completed bool
}

// RootRejected reports whether the filter's root node evaluated to false.
func (b BranchHits) RootRejected() bool {
	for _, n := range b.Filter {
		if n.Node == string(rootNode) {
			return !n.Result
		}
	}
	return false
}

// StepIndex returns the position of the executed step with id, if any.
func (b BranchHits) StepIndex(id string) (int, bool) {
	for i, s := range b.Steps {
		if s.ID == id {
			return i, true
		}
	}
	return 0, false
}

// FilterRoot names the relative pointer of a branch filter's root node.
type FilterRoot string

const rootNode FilterRoot = ""

// PreviewTracer implements the engine's trace.Tracer for one event, diffing step snapshots
// with the preview PatchDiffer (add/remove/replace only).
type PreviewTracer struct {
	differ   PatchDiffer
	branches []*BranchHits
	index    map[string]int
	status   trace.EventStatus
	err      *trace.ErrorInfo
}

// NewPreviewTracer starts a trace using differ.
func NewPreviewTracer(differ PatchDiffer) *PreviewTracer {
	return &PreviewTracer{differ: differ, index: map[string]int{}}
}

func (t *PreviewTracer) Enabled() bool { return true }

func (t *PreviewTracer) Node(branch, nodeID string, ok bool, warning string) {
	b := t.branch(branch)
	b.Filter = append(b.Filter, NodeHit{Node: nodeID, Result: ok, Warning: warning})
}

func (t *PreviewTracer) Step(r trace.StepRecord) {
	b := t.branch(r.Branch)
	logs := r.Logs
	if logs == nil {
		logs = []string{}
	}
	b.Steps = append(b.Steps, StepHit{ID: r.StepID, Kind: r.Kind, Patch: t.patch(r.Before, r.After), Duration: r.Duration, Logs: logs, Err: r.Err})
}

func (t *PreviewTracer) Outputs(branch string, n int) {
	b := t.branch(branch)
	b.Outputs, b.Completed = n, true
}

func (t *PreviewTracer) Finish(status trace.EventStatus, err *trace.ErrorInfo) {
	t.status, t.err = status, err
}

// Branches returns the traced branches in first-seen order.
func (t *PreviewTracer) Branches() []BranchHits {
	out := make([]BranchHits, 0, len(t.branches))
	for _, b := range t.branches {
		out = append(out, *b)
	}
	return out
}

// Status returns the engine's final status for the event.
func (t *PreviewTracer) Status() trace.EventStatus { return t.status }

// patch diffs one-to-one snapshots directly and fan-outs as lists.
func (t *PreviewTracer) patch(before, after []any) []trace.PatchOp {
	if len(before) == 1 && len(after) == 1 {
		return t.differ.Diff(before[0], after[0])
	}
	return t.differ.Diff(before, after)
}

func (t *PreviewTracer) branch(id string) *BranchHits {
	i, ok := t.index[id]
	if !ok {
		t.branches = append(t.branches, &BranchHits{ID: id, Filter: []NodeHit{}, Steps: []StepHit{}})
		i = len(t.branches) - 1
		t.index[id] = i
	}
	return t.branches[i]
}
