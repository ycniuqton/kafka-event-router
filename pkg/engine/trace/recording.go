package trace

// RecordingTracer builds an EventTrace for one event (preview, tests).
type RecordingTracer struct {
	patches PatchBuilder
	out     EventTrace
	index   map[string]int
}

// NewRecordingTracer starts a trace for the event at offset.
func NewRecordingTracer(offset int64) *RecordingTracer {
	return &RecordingTracer{out: EventTrace{Offset: offset, Branches: []BranchTrace{}}, index: map[string]int{}}
}

func (t *RecordingTracer) Enabled() bool { return true }

func (t *RecordingTracer) Node(branch, nodeID string, ok bool, warning string) {
	b := t.branch(branch)
	b.Filter = append(b.Filter, NodeResult{Node: nodeID, Result: ok, Warning: warning})
}

func (t *RecordingTracer) Step(r StepRecord) {
	b := t.branch(r.Branch)
	st := StepTrace{ID: r.StepID, Kind: r.Kind, DurationUs: r.Duration.Microseconds(), Logs: r.Logs}
	if st.Logs == nil {
		st.Logs = []string{}
	}
	st.Patch = t.patches.Diff(r.Before, r.After)
	if r.Err != nil {
		msg := r.Err.Error()
		st.Error = &msg
	}
	b.Steps = append(b.Steps, st)
}

func (t *RecordingTracer) Outputs(branch string, n int) {
	t.branch(branch).Outputs = n
}

func (t *RecordingTracer) Finish(status EventStatus, err *ErrorInfo) {
	t.out.Status = status
	t.out.Error = err
}

// Trace returns the recorded trace.
func (t *RecordingTracer) Trace() EventTrace { return t.out }

func (t *RecordingTracer) branch(id string) *BranchTrace {
	i, ok := t.index[id]
	if !ok {
		t.out.Branches = append(t.out.Branches, BranchTrace{ID: id, Filter: []NodeResult{}, Steps: []StepTrace{}})
		i = len(t.out.Branches) - 1
		t.index[id] = i
	}
	return &t.out.Branches[i]
}
