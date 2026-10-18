package preview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/engine"
)

// OutputValidator encodes outputs through the pipeline's destination codecs and limits.
type OutputValidator struct{}

// ValidatedOutputs are the rendered outputs per branch plus the encoded records that validated.
type ValidatedOutputs struct {
	ByBranch map[string][]OutputDTO
	Encoded  []engine.RawRecord
}

// Validate encodes every output; failures are reported per output and as output_invalid.
func (OutputValidator) Validate(ctx context.Context, p engine.Pipeline, outs []engine.Out) (ValidatedOutputs, error) {
	byBranch := map[string][]OutputDTO{}
	encoded := []engine.RawRecord{}
	var firstErr error
	for _, o := range outs {
		raw, err := p.Encode(ctx, o)
		dto := OutputDTO{Topic: raw.Topic, Valid: err == nil, Headers: []engine.HeaderDoc{}}
		if err != nil {
			msg := err.Error()
			dto.ValidationError = &msg
			dto.Key, dto.Value = o.Key, o.Value
			firstErr = OutputValidator{}.first(firstErr, err, o.Branch)
		} else {
			dto.Key, dto.Value = BytesRenderer{}.Value(raw.Key), BytesRenderer{}.Value(raw.Value)
			dto.SizeBytes = len(raw.Key) + len(raw.Value)
			dto.Headers = BytesRenderer{}.Headers(raw.Headers)
			encoded = append(encoded, raw)
		}
		byBranch[string(o.Branch)] = append(byBranch[string(o.Branch)], dto)
	}
	return ValidatedOutputs{ByBranch: byBranch, Encoded: encoded}, firstErr
}

func (OutputValidator) first(prev, err error, branch engine.BranchID) error {
	if prev != nil {
		return prev
	}
	return &engine.ProcessError{Type: engine.ErrOutputInvalid, Branch: branch, Cause: err}
}

// BytesRenderer renders record bytes for DTOs: JSON as JSON, UTF-8 text as string, else base64.
type BytesRenderer struct{}

// Value renders b (nil stays nil).
func (BytesRenderer) Value(b []byte) any {
	if b == nil {
		return nil
	}
	if json.Valid(b) {
		return json.RawMessage(append([]byte(nil), b...))
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// Text renders b as text with the encoding used.
func (BytesRenderer) Text(b []byte) (string, RawEncoding) {
	if utf8.Valid(b) {
		return string(b), EncodingUTF8
	}
	return base64.StdEncoding.EncodeToString(b), EncodingBase64
}

// Headers renders headers in the event JSON shape.
func (BytesRenderer) Headers(hs []engine.Header) []engine.HeaderDoc {
	out := make([]engine.HeaderDoc, 0, len(hs))
	for _, h := range hs {
		out = append(out, engine.HeaderDoc{Key: h.Key, Value: string(h.Value)})
	}
	return out
}
