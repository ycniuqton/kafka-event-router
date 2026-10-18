package trace

import "encoding/json"

// EventTrace is the per-event trace JSON returned by POST /preview/run.
type EventTrace struct {
	Offset   int64         `json:"offset"`
	Status   EventStatus   `json:"status"`
	Branches []BranchTrace `json:"branches"`
	Error    *ErrorInfo    `json:"error"`
}

// BranchTrace holds the filter and step diagnostics of one branch.
type BranchTrace struct {
	ID      string       `json:"id"`
	Filter  []NodeResult `json:"filter"`
	Steps   []StepTrace  `json:"steps"`
	Outputs int          `json:"outputs"`
}

// NodeResult is one evaluated filter node (pointer relative to the branch filter).
type NodeResult struct {
	Node    string `json:"node"`
	Result  bool   `json:"result"`
	Warning string `json:"warning,omitempty"`
}

// StepTrace is one step: patch against the previous step's output, timing, logs, error.
type StepTrace struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Patch      []PatchOp `json:"patch"`
	DurationUs int64     `json:"durationUs"`
	Logs       []string  `json:"logs"`
	Error      *string   `json:"error"`
}

// PatchOp is one RFC 6902 operation.
type PatchOp struct {
	Op       string
	From     string
	Path     string
	Value    any
	HasValue bool
}

// MarshalJSON writes value whenever the operation carries one, including null.
func (p PatchOp) MarshalJSON() ([]byte, error) {
	m := map[string]any{"op": p.Op, "path": p.Path}
	if p.From != "" {
		m["from"] = p.From
	}
	if p.HasValue {
		m[string(PatchMemberValue)] = p.Value
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads an operation, remembering whether a value member was present.
func (p *PatchOp) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	var op struct {
		Op   string `json:"op"`
		From string `json:"from"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(b, &op); err != nil {
		return err
	}
	*p = PatchOp{Op: op.Op, From: op.From, Path: op.Path}
	raw, ok := m[string(PatchMemberValue)]
	if !ok {
		return nil
	}
	p.HasValue = true
	return json.Unmarshal(raw, &p.Value)
}

// PatchMember names RFC 6902 operation members.
type PatchMember string

const PatchMemberValue PatchMember = "value"
