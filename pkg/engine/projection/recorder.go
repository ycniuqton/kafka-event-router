// Package projection runs JSON routes on a partial decode: at compile time it learns which
// value fields a route can touch, at run time it decodes only those fields and patches
// the changes back into the original bytes (byte-identical to the tree codec).
package projection

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/path"
)

// Recorder collects, during one route compile, everything the analyser needs: parsed
// paths, compiled component kinds and uses it cannot see through. Nil-safe.
type Recorder struct {
	Paths    []path.FieldPath
	Steps    []config.StepKind
	Nodes    []config.NodeType
	Sources  []config.ValueSourceKind
	Scripts  []string
	Formulas []string
}

// NewRecorder returns an empty recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// Path records a parsed field path.
func (r *Recorder) Path(p path.FieldPath) {
	if r != nil {
		r.Paths = append(r.Paths, p)
	}
}

// Step records a compiled step kind.
func (r *Recorder) Step(k config.StepKind) {
	if r != nil {
		r.Steps = append(r.Steps, k)
	}
}

// Node records a compiled filter node type.
func (r *Recorder) Node(t config.NodeType) {
	if r != nil {
		r.Nodes = append(r.Nodes, t)
	}
}

// Source records a compiled value source kind.
func (r *Recorder) Source(k config.ValueSourceKind) {
	if r != nil {
		r.Sources = append(r.Sources, k)
	}
}

// Script records user code (js step, expr node) at origin; its field use is unknown.
func (r *Recorder) Script(origin string) {
	if r != nil {
		r.Scripts = append(r.Scripts, origin)
	}
}

// Formula records the fields of a compiled formula, or the formula itself when its
// fields cannot be listed.
func (r *Recorder) Formula(src string, fields []path.FieldPath, ok bool) {
	if r == nil {
		return
	}
	if !ok {
		r.Formulas = append(r.Formulas, src)
		return
	}
	r.Paths = append(r.Paths, fields...)
}
