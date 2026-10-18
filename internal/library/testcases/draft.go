package testcases

import (
	"encoding/json"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/config"
)

// DraftResolver picks the document a run compiles: the request's config, else the route's stored
// draft with the first branch's steps replaced by the request's steps (the Studio's unsaved edit).
type DraftResolver struct{}

// Document returns the route document of the run (422 with the request member's path).
func (r DraftResolver) Document(req RunRequest, route RouteInfo) (json.RawMessage, error) {
	if len(req.Config) > 0 && string(req.Config) != nullJSON {
		if !r.isObject(req.Config) {
			return nil, r.invalid(FieldConfig, MsgDraftInvalid)
		}
		return req.Config, nil
	}
	if len(req.Steps) == 0 || string(req.Steps) == nullJSON {
		return nil, r.invalid(FieldConfig, MsgDraftRequired)
	}
	if len(route.Draft) == 0 {
		return nil, r.invalid(FieldSteps, MsgNoDraft)
	}
	doc, err := StepsMerger{}.Merge(route.Draft, req.Steps)
	if err != nil {
		return nil, r.invalid(FieldSteps, MsgDraftInvalid)
	}
	return doc, nil
}

func (DraftResolver) isObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m != nil
}

func (DraftResolver) invalid(f Field, m Message) error {
	return api.NewValidationError().Add(string(f), string(CodeInvalid), string(m))
}

// StepsMerger replaces the steps of the first branch of a route document.
type StepsMerger struct{}

// Merge returns doc with branches[0].transform.steps = steps (no transform when steps is empty).
func (StepsMerger) Merge(doc, steps json.RawMessage) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(doc, &root); err != nil {
		return nil, err
	}
	var branches []map[string]json.RawMessage
	if err := json.Unmarshal(root[string(config.TokenBranches)], &branches); err != nil || len(branches) == 0 {
		return nil, ErrBranchMissing
	}
	var list []json.RawMessage
	if err := json.Unmarshal(steps, &list); err != nil {
		return nil, err
	}
	transform := map[string]json.RawMessage{}
	_ = json.Unmarshal(branches[0][string(config.TokenTransform)], &transform)
	transform[string(config.TokenSteps)] = steps
	encoded, err := json.Marshal(transform)
	if err != nil {
		return nil, err
	}
	branches[0][string(config.TokenTransform)] = encoded
	if len(list) == 0 {
		delete(branches[0], string(config.TokenTransform))
	}
	if root[string(config.TokenBranches)], err = json.Marshal(branches); err != nil {
		return nil, err
	}
	return json.Marshal(root)
}
