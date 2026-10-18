package preview

import (
	"encoding/json"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// ResponseMasker masks every payload-derived value of preview responses before they leave the service:
// sample values, keys, headers and raw text, step patches and logs, outputs and schema top values.
type ResponseMasker struct {
	masker ValueMasker
}

// NewResponseMasker masks with the given value masker.
func NewResponseMasker(m ValueMasker) ResponseMasker { return ResponseMasker{masker: m} }

// Samples masks a page of samples.
func (r ResponseMasker) Samples(in []SampleDTO) []SampleDTO {
	out := make([]SampleDTO, len(in))
	for i, s := range in {
		out[i] = r.Sample(s)
	}
	return out
}

// Sample masks one sample; raw text becomes the JSON of the masked value (or masked text when undecodable).
func (r ResponseMasker) Sample(s SampleDTO) SampleDTO {
	s.Key = r.masker.Mask(s.Key)
	s.Value = r.masker.Mask(s.Value)
	s.Headers = r.Headers(s.Headers)
	s.Raw = r.raw(s)
	return s
}

func (r ResponseMasker) raw(s SampleDTO) RawDTO {
	out := RawDTO{Encoding: EncodingUTF8, Value: r.masker.MaskText(s.Raw.Value)}
	if s.DecodeError == nil {
		if b, err := json.Marshal(s.Value); err == nil {
			out.Value = string(b)
		}
	}
	if s.Raw.Key != nil {
		key := r.masker.MaskText(*s.Raw.Key)
		out.Key = &key
	}
	return out
}

// Headers masks header values and keeps header names.
func (r ResponseMasker) Headers(in []engine.HeaderDoc) []engine.HeaderDoc {
	out := make([]engine.HeaderDoc, len(in))
	for i, h := range in {
		out[i] = engine.HeaderDoc{Key: h.Key, Value: r.masker.MaskText(h.Value)}
	}
	return out
}

// Run masks every event result of a run.
func (r ResponseMasker) Run(res RunResultDTO) RunResultDTO {
	results := make([]EventResultDTO, len(res.Results))
	for i, ev := range res.Results {
		ev.Branches = r.branches(ev.Branches)
		results[i] = ev
	}
	res.Results, res.Masked = results, true
	return res
}

func (r ResponseMasker) branches(in []BranchResultDTO) []BranchResultDTO {
	out := make([]BranchResultDTO, len(in))
	for i, b := range in {
		b.Steps = r.steps(b.Steps)
		b.Outputs = r.outputs(b.Outputs)
		out[i] = b
	}
	return out
}

func (r ResponseMasker) steps(in []StepTraceDTO) []StepTraceDTO {
	out := make([]StepTraceDTO, len(in))
	for i, s := range in {
		s.Patch = r.patch(s.Patch)
		s.Logs = r.texts(s.Logs)
		out[i] = s
	}
	return out
}

// patch masks step patch values; header names stay, as in samples and outputs.
func (r ResponseMasker) patch(in []trace.PatchOp) []trace.PatchOp {
	ops := PatchMasker{masker: r.masker}
	out := make([]trace.PatchOp, len(in))
	for i, op := range in {
		out[i] = ops.Op(op)
	}
	return out
}

func (r ResponseMasker) outputs(in []OutputDTO) []OutputDTO {
	out := make([]OutputDTO, len(in))
	for i, o := range in {
		o.Key = r.masker.Mask(o.Key)
		o.Value = r.masker.Mask(o.Value)
		o.Headers = r.Headers(o.Headers)
		out[i] = o
	}
	return out
}

func (r ResponseMasker) texts(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = r.masker.MaskText(s)
	}
	return out
}

// Schema masks the top values of every field (counts and frequencies stay).
func (r ResponseMasker) Schema(fs FieldSchema) FieldSchema {
	fs.Root = r.node(fs.Root)
	return fs
}

func (r ResponseMasker) node(n FieldNode) FieldNode {
	values := make([]ValueCount, len(n.TopValues))
	for i, v := range n.TopValues {
		v.Value = r.masker.Mask(v.Value)
		values[i] = v
	}
	children := make([]FieldNode, len(n.Children))
	for i, c := range n.Children {
		children[i] = r.node(c)
	}
	n.TopValues, n.Children = values, children
	return n
}

// MaskPolicy decides whether a response is masked: the caller's directive or the session's load decision.
type MaskPolicy struct {
	Maskers *MaskerManager
}

// For returns the masker to apply and whether masking is on.
func (p MaskPolicy) For(caller Caller, sess Session) (ResponseMasker, bool) {
	masked := caller.Mask || sess.Masked
	style := caller.MaskStyle
	if sess.Masked && sess.MaskStyle != "" {
		style = sess.MaskStyle
	}
	return NewResponseMasker(p.registry().For(style)), masked
}

// registry returns the configured maskers, the default set when none were wired.
func (p MaskPolicy) registry() *MaskerManager {
	if p.Maskers == nil {
		return DefaultMaskerManager()
	}
	return p.Maskers
}
