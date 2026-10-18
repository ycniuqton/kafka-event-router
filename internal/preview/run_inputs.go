package preview

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// InlineTopic names the topic given to inline run events without one.
type InlineTopic string

const InlineEventsTopic InlineTopic = "inline"

// RunInputs resolves what a run processes: session records (optionally a subset) or inline events.
type RunInputs struct {
	Sessions *SessionStoreManager
	Now      func() time.Time
}

// Plan validates the request and builds the run plan; session runs slide the session TTL.
func (in RunInputs) Plan(ctx context.Context, caller Caller, req RunRequest, mode RunMode) (RunPlan, error) {
	if (req.SessionID == "") == (len(req.Events) == 0) {
		return RunPlan{}, ErrRunInput.At(string(FieldSessionID))
	}
	if len(req.Config) == 0 {
		return RunPlan{}, ErrRunInput.At(string(FieldConfig))
	}
	plan := RunPlan{Config: req.Config, Scope: RunScope{TeamID: caller.TeamID, Mode: mode}}
	if req.SessionID == "" {
		return in.inline(ctx, plan, caller, req)
	}
	return in.session(ctx, plan, caller, req, mode)
}

// inline converts inline events; when they were copied from a session, that session's masking decision applies.
func (in RunInputs) inline(ctx context.Context, plan RunPlan, caller Caller, req RunRequest) (RunPlan, error) {
	if len(req.Events) > int(MaxInlineEvents) {
		return RunPlan{}, ErrTooManyEvents.At(string(FieldEvents))
	}
	if req.SampleSessionID != "" {
		sess, err := in.Sessions.Owned(ctx, req.SampleSessionID, caller)
		if err != nil {
			return RunPlan{}, err
		}
		plan.Session = sess
	}
	conv := InlineEventConverter{Format: RawConfigFormat{}.Of(req.Config), Topic: string(InlineEventsTopic), Now: in.Now().UTC()}
	for i, ev := range req.Events {
		rec, err := conv.Record(i, ev)
		if err != nil {
			return RunPlan{}, ErrUploadItem.At(FieldEvents.Indexed(i)).Wrap(err)
		}
		plan.Records = append(plan.Records, rec)
	}
	plan.Loaded = len(plan.Records)
	return plan, nil
}

func (in RunInputs) session(ctx context.Context, plan RunPlan, caller Caller, req RunRequest, mode RunMode) (RunPlan, error) {
	sess, err := in.Sessions.Owned(ctx, req.SessionID, caller)
	if err != nil {
		return RunPlan{}, err
	}
	recs, err := in.Sessions.All(ctx, req.SessionID)
	if err != nil {
		return RunPlan{}, err
	}
	if _, err := in.Sessions.Touch(ctx, sess); err != nil {
		return RunPlan{}, err
	}
	if plan.Scope.TeamID == "" {
		plan.Scope.TeamID = sess.TeamID
	}
	plan.Loaded, plan.Session = len(recs), sess
	plan.Records = SeqSelection{}.Pick(recs, req.EventSeqs, mode)
	return plan, nil
}

// SeqSelection narrows session records to requested seqs (preview runs only; tests run all).
type SeqSelection struct{}

// Pick returns the records whose seq is listed, or all of them.
func (SeqSelection) Pick(recs []SampleRecord, seqs []int, mode RunMode) []SampleRecord {
	if len(seqs) == 0 || mode == RunTest {
		return recs
	}
	want := map[int]bool{}
	for _, s := range seqs {
		want[s] = true
	}
	out := make([]SampleRecord, 0, len(seqs))
	for _, r := range recs {
		if want[r.Seq] {
			out = append(out, r)
		}
	}
	return out
}

// RawConfigFormat reads the source value format of a draft without full validation.
type RawConfigFormat struct{}

// Of returns source.format.type of raw, JSON when absent or unreadable.
func (RawConfigFormat) Of(raw RawConfig) config.FormatType {
	var doc struct {
		Source struct {
			Format config.Format `json:"format"`
		} `json:"source"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Source.Format.Type == "" {
		return config.FormatTypeJSON
	}
	return doc.Source.Format.Type
}

// DraftRefs are the connections and route name a draft refers to.
type DraftRefs struct {
	Name         string
	Source       string
	Destinations []string
}

// Destination returns the destination connection of the first branch ("" when none).
func (d DraftRefs) Destination() string {
	if len(d.Destinations) == 0 {
		return ""
	}
	return d.Destinations[0]
}

// DraftRefReader reads DraftRefs from a raw draft without full validation.
type DraftRefReader struct{}

// Of returns the refs of raw; unreadable drafts have none.
func (DraftRefReader) Of(raw []byte) DraftRefs {
	var doc struct {
		Name   string `json:"name"`
		Source struct {
			ConnectionID string `json:"connectionId"`
		} `json:"source"`
		Branches []struct {
			Destination struct {
				ConnectionID string `json:"connectionId"`
			} `json:"destination"`
		} `json:"branches"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return DraftRefs{}
	}
	refs := DraftRefs{Name: doc.Name, Source: doc.Source.ConnectionID}
	for _, b := range doc.Branches {
		refs.Destinations = append(refs.Destinations, b.Destination.ConnectionID)
	}
	return refs
}
