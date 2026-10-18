package testcaseshttp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/eventrouter/router/internal/preview"
)

// RawVisibility decides whether the caller may see raw event values of a team's routes
// (access-control: Viewers never; implemented in cmd/api over sample.read_raw).
type RawVisibility interface {
	RawAllowed(ctx context.Context, teamID string) bool
}

// CaseMasking masks pinned events and outputs for callers without raw access (type-preserving masker).
type CaseMasking struct {
	Visibility RawVisibility
	Masker     preview.ValueMasker
}

// For returns the masker to apply for the request (nil: show raw values).
func (m CaseMasking) For(r *http.Request, team string) *DocMasker {
	if m.Visibility == nil || m.Masker == nil || m.Visibility.RawAllowed(r.Context(), team) {
		return nil
	}
	return &DocMasker{masker: m.Masker}
}

// DocMasker masks raw JSON documents leaf by leaf (object keys and shapes are kept).
type DocMasker struct {
	masker preview.ValueMasker
}

// Doc returns the masked document (null and invalid JSON stay as they are).
func (d *DocMasker) Doc(raw json.RawMessage) json.RawMessage {
	if d == nil || len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return raw
	}
	out, err := json.Marshal(d.masker.Mask(v))
	if err != nil {
		return raw
	}
	return out
}

// Case masks the input and expected output of a case DTO.
func (d *DocMasker) Case(c CaseDTO) CaseDTO {
	c.InputEvent, c.ExpectedOutput = d.Doc(c.InputEvent), d.Doc(c.ExpectedOutput)
	return c
}

// Report masks actual outputs and diff values of a run report.
func (d *DocMasker) Report(rep ReportDTO) ReportDTO {
	if d == nil {
		return rep
	}
	for i := range rep.Results {
		rep.Results[i].ActualOutput = d.Doc(rep.Results[i].ActualOutput)
		for j := range rep.Results[i].Diff {
			if rep.Results[i].Diff[j].HasValue {
				rep.Results[i].Diff[j].Value = d.masker.Mask(rep.Results[i].Diff[j].Value)
			}
		}
	}
	return rep
}
