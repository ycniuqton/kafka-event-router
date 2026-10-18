package dlq

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/pkg/engine"
)

// StudioLink tells the browser where the builder opens with the failing event as sample.
type StudioLink struct {
	SessionID   string `json:"sessionId"`
	SampleIndex int    `json:"sampleIndex"`
	BuilderURL  string `json:"builderUrl"`
}

// StudioLinker creates a sample session holding one dead letter (FR-18).
type StudioLinker interface {
	Link(ctx context.Context, route RouteInfo, viewer Viewer, rec record.Record, masked bool) (StudioLink, error)
}

// PreviewForwarder is the port to the preview service (gateway.PreviewClient).
type PreviewForwarder interface {
	Forward(ctx context.Context, method, path string, body []byte, caller preview.Caller) (gateway.Upstream, error)
}

// BuilderLink holds the builder deep-link parts.
type BuilderLink string

const (
	builderPath  BuilderLink = "/routes/"
	builderEdit  BuilderLink = "/edit"
	stepParam    BuilderLink = "step"
	stepValue    BuilderLink = "transform"
	sessionParam BuilderLink = "session"
	sampleParam  BuilderLink = "sample"
)

// PreviewStudioLinker uploads the event as an inline sample session through the preview service.
type PreviewStudioLinker struct {
	Preview PreviewForwarder
}

// Link implements StudioLinker; masked records are uploaded masked under a masking directive; preview failures are 503.
func (l PreviewStudioLinker) Link(ctx context.Context, route RouteInfo, viewer Viewer, rec record.Record, masked bool) (StudioLink, error) {
	body, _ := json.Marshal(preview.LoadRequest{Mode: preview.ModeUpload, Topics: []string{rec.Source.Topic}, Events: []preview.InlineEvent{(InlineOf{}).Event(rec)}})
	ctx, cancel := context.WithTimeout(ctx, StudioCallLimit.Duration())
	defer cancel()
	up, err := l.Preview.Forward(ctx, http.MethodPost, preview.PrefixInternal.Pattern(preview.EndpointSamples), body, (CallerOf{}).For(route, viewer, masked))
	if err != nil || !up.OK() {
		return StudioLink{}, ErrStudioDown.Wrap(err)
	}
	var resp preview.LoadResponseDTO
	if err := json.Unmarshal(up.Body, &resp); err != nil || resp.Session.ID == "" {
		return StudioLink{}, ErrStudioDown.Wrap(err)
	}
	return (StudioLinkOf{}).Build(route.ID, string(resp.Session.ID)), nil
}

// CallerOf builds preview directives.
type CallerOf struct{}

// For returns the caller of viewer on route; masked sessions use the type-preserving style.
func (CallerOf) For(route RouteInfo, viewer Viewer, masked bool) preview.Caller {
	c := preview.Caller{UserID: viewer.UserID, TeamID: route.TeamID, Mask: masked}
	if masked {
		c.MaskStyle = preview.MaskTypePreserving
	}
	return c
}

// StudioLinkOf builds builder deep links.
type StudioLinkOf struct{}

// Build returns the link to the transform step with sample 0 of session.
func (StudioLinkOf) Build(routeID, session string) StudioLink {
	q := url.Values{}
	q.Set(string(stepParam), string(stepValue))
	q.Set(string(sessionParam), session)
	q.Set(string(sampleParam), strconv.Itoa(0))
	return StudioLink{SessionID: session, SampleIndex: 0, BuilderURL: string(builderPath) + url.PathEscape(routeID) + string(builderEdit) + "?" + q.Encode()}
}

// InlineOf converts a dead letter to a preview inline event.
type InlineOf struct{}

// Event keeps JSON bytes as JSON and wraps other bytes as JSON strings.
func (InlineOf) Event(rec record.Record) preview.InlineEvent {
	ts := rec.Failure.FailedAt
	p, o := rec.Source.Partition, rec.Source.Offset
	hs := make([]engine.HeaderDoc, 0, len(rec.Headers))
	for _, h := range rec.Headers {
		hs = append(hs, engine.HeaderDoc{Key: h.Key, Value: string(h.Value)})
	}
	return preview.InlineEvent{Key: InlineOf{}.json(rec.Key), Value: InlineOf{}.json(rec.Value), Headers: hs, Timestamp: &ts, Topic: rec.Source.Topic, Partition: &p, Offset: &o}
}

func (InlineOf) json(b []byte) json.RawMessage {
	if b == nil {
		return nil
	}
	if json.Valid(b) {
		return b
	}
	raw, _ := json.Marshal(string(b))
	return raw
}
