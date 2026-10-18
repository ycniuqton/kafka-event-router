package preview

import "encoding/json"

// SampleValueMasker masks raw JSON sample values for features outside the preview service (AI assist
// prompts, LP-202) with the same maskers as preview responses; invalid JSON is masked as text.
type SampleValueMasker struct {
	Maskers *MaskerManager
}

// NewSampleValueMasker returns the masker over the default style set (type-preserving by default).
func NewSampleValueMasker() SampleValueMasker {
	return SampleValueMasker{Maskers: DefaultMaskerManager()}
}

// Mask returns a masked copy of every sample in style (unknown or empty style = default).
func (m SampleValueMasker) Mask(style MaskStyle, samples []json.RawMessage) []json.RawMessage {
	masker := m.registry().For(style)
	out := make([]json.RawMessage, 0, len(samples))
	for _, s := range samples {
		out = append(out, m.one(masker, s))
	}
	return out
}

func (m SampleValueMasker) one(masker ValueMasker, sample json.RawMessage) json.RawMessage {
	b, err := json.Marshal(masker.Mask(sample))
	if err != nil {
		b, _ = json.Marshal(masker.MaskText(string(sample)))
	}
	return b
}

func (m SampleValueMasker) registry() *MaskerManager {
	if m.Maskers == nil {
		return DefaultMaskerManager()
	}
	return m.Maskers
}
