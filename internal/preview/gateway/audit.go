package gateway

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/preview"
)

// SampleViewRecord is the audited summary of one sample load (never payloads).
type SampleViewRecord struct {
	ConnectionID string             `json:"connectionId,omitempty"`
	Topics       []string           `json:"topics"`
	Mode         preview.SampleMode `json:"mode"`
	Loaded       int                `json:"loaded"`
	Masked       bool               `json:"masked"`
}

// SampleAuditor records sample.viewed (or sample.viewed_raw) for every successful load.
type SampleAuditor struct{}

// loadResponse is the part of the load response the audit entry needs.
type loadResponse struct {
	Session preview.SessionDTO `json:"session"`
}

// Record writes the entry through the request's audit scope.
func (SampleAuditor) Record(ctx context.Context, p auth.Principal, req preview.LoadRequest, body []byte) error {
	scope, ok := audit.FromContext(ctx)
	if !ok {
		return nil
	}
	var resp loadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	action := audit.ActionSampleViewedRaw
	if resp.Session.Masked {
		action = audit.ActionSampleViewed
	}
	rec := SampleViewRecord{ConnectionID: req.ConnectionID, Topics: req.Topics, Mode: req.Mode, Loaded: resp.Session.Loaded, Masked: resp.Session.Masked}
	return scope.Record(ctx, audit.Change{
		Action: action, Object: audit.ObjectSampleSession, ObjectID: string(resp.Session.ID), TeamID: SampleAuditor{}.team(p), After: rec,
	})
}

func (SampleAuditor) team(p auth.Principal) *uuid.UUID {
	if len(p.Teams) == 0 {
		return nil
	}
	id := p.Teams[0].TeamID
	return &id
}

// ActionTestTopicWritten is the audit action of a test run that wrote outputs to a test topic
// (registered in access-control's catalog at wiring time).
const ActionTestTopicWritten audit.Action = "preview.test_topic_written"

// TestTopicRecord is the audited summary of a test-topic write.
type TestTopicRecord struct {
	ConnectionID string `json:"connectionId"`
	Topic        string `json:"topic"`
	Produced     int    `json:"produced"`
}

// TestTopicAuditor records ActionTestTopicWritten when the test-run response names a test topic;
// test runs are not mutating routes, so it writes through the recorder directly (diff = created record).
type TestTopicAuditor struct {
	Recorder audit.Recorder
}

// Record writes the entry for the request in rc.
func (a TestTopicAuditor) Record(ctx context.Context, rc audit.RequestContext, body []byte) error {
	var resp struct {
		TestTopic *preview.TestTopicDTO `json:"testTopic"`
	}
	if a.Recorder == nil {
		return nil
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.TestTopic == nil {
		return err
	}
	t := resp.TestTopic
	diff, err := json.Marshal(TestTopicRecord{ConnectionID: t.ConnectionID, Topic: t.Topic, Produced: t.Produced})
	if err != nil {
		return err
	}
	return a.Recorder.Record(ctx, rc.Apply(audit.Entry{Action: ActionTestTopicWritten, Object: audit.ObjectTopic, ObjectID: t.Topic, Diff: diff}))
}
