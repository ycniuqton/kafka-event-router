package dlq

import (
	"strings"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// EditInput is what an edit is planned from: the stored record, the requested edit, whether the editor saw
// the record masked, and the source format of the version the replay runs through.
type EditInput struct {
	Original record.Record
	Edit     *RecordEdit
	Masked   bool
	Format   SourceFormat
}

// EditPlanner validates an edit, restores untouched masked values and computes the structural diff.
type EditPlanner struct {
	Masker ValueMasker
}

// Plan returns the record to replay; an empty edit replays the original unchanged.
func (p EditPlanner) Plan(in EditInput) (EditPlan, error) {
	if in.Edit.Empty() {
		return EditPlan{Record: in.Original, Diff: []DiffEntry{}, Masked: in.Masked}, nil
	}
	rec := in.Original
	var err error
	if rec.Key, err = p.part(in.Edit.Key, in.Original.Key, in.Format.Key, PathEditKey, in.Masked); err != nil {
		return EditPlan{}, err
	}
	if rec.Value, err = p.part(in.Edit.Value, in.Original.Value, in.Format.Value, PathEditValue, in.Masked); err != nil {
		return EditPlan{}, err
	}
	rec.Headers = p.headers(in.Edit.Headers, in.Original.Headers, in.Masked)
	if rec.Size() > int64(MaxEditBytes) {
		return EditPlan{}, ErrEditTooBig.At(string(PathEdit))
	}
	diff := (RecordDiff{}).Of(in.Original, rec)
	return EditPlan{Record: rec, Diff: diff, Masked: in.Masked, Edited: len(diff) > 0}, nil
}

// part returns the bytes of one edited member (nil edit keeps the original).
func (p EditPlanner) part(e *EncodedValue, original []byte, format config.Format, path RequestPath, masked bool) ([]byte, error) {
	if e == nil {
		return original, nil
	}
	b, err := EditBytes{}.Of(*e, path)
	if err != nil {
		return nil, err
	}
	if masked {
		b = MaskMerger{Masker: p.Masker}.Bytes(b, original)
	}
	return b, EditFormatCheck{}.Check(format, b, path)
}

// headers returns the edited headers without router.* keys; masked values left unchanged are restored.
func (p EditPlanner) headers(edited *[]EncodedHeader, original []engine.Header, masked bool) []engine.Header {
	if edited == nil {
		return original
	}
	orig := HeaderTree{}.Of(original)
	out := make([]engine.Header, 0, len(*edited))
	for _, h := range *edited {
		if strings.HasPrefix(h.Key, string(record.RouterNamespace)) || h.Key == "" {
			continue
		}
		out = append(out, engine.Header{Key: h.Key, Value: p.headerValue(h, orig, masked)})
	}
	return out
}

func (p EditPlanner) headerValue(h EncodedHeader, orig map[string]any, masked bool) []byte {
	value, _ := EditBytes{}.Of(EncodedValue{Encoding: (HeaderEncodings{}).Of(h.Encoding), Data: h.Value}, PathEditHeaders)
	if prev, ok := orig[h.Key].(string); ok && masked {
		return MaskMerger{Masker: p.Masker}.Text(value, []byte(prev))
	}
	return value
}

// HeaderEncodings maps header encodings (utf8 default, base64 for binary headers).
type HeaderEncodings struct{}

// Of returns base64 for base64 headers, else utf8.
func (HeaderEncodings) Of(e Encoding) Encoding {
	if e == EncodingBase64 {
		return EncodingBase64
	}
	return EncodingUTF8
}

// RecordDiff compares a record before and after an edit.
type RecordDiff struct{}

// Of returns the diff entries of key, value and headers.
func (RecordDiff) Of(before, after record.Record) []DiffEntry {
	d, t := StructuralDiff{}, JSONTree{}
	out := []DiffEntry{}
	out = append(out, d.Of("/key", t.Of(before.Key), t.Of(after.Key))...)
	out = append(out, d.Of("/value", t.Of(before.Value), t.Of(after.Value))...)
	return append(out, d.Of("/headers", HeaderTree{}.Of(before.Headers), HeaderTree{}.Of(after.Headers))...)
}
