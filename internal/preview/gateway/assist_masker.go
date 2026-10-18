package gateway

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/preview"
)

// MaskedSamples are sample values safe to hand to another feature, and whether they were masked.
type MaskedSamples struct {
	Samples []json.RawMessage
	Masked  bool
}

// AssistSampleMasker lets AI assist (transform-studio) send samples to an LLM under the same rule as the
// sample panel: access-control SampleVisibility decides, the preview maskers mask (fail closed = masked).
type AssistSampleMasker struct {
	Authorizer PreviewAuthorizer
	Values     preview.SampleValueMasker
}

// Mask decides masking for p reading samples of connectionID ("" = own data) and masks when required
// (an unknown connection is masked too).
func (m AssistSampleMasker) Mask(ctx context.Context, p auth.Principal, connectionID string, samples []json.RawMessage) MaskedSamples {
	caller, err := m.Authorizer.Caller(ctx, p, connectionID)
	if err == nil && !caller.Mask {
		return MaskedSamples{Samples: samples}
	}
	return MaskedSamples{Samples: m.Values.Mask(caller.MaskStyle, samples), Masked: true}
}
