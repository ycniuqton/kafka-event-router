package connhttp

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/httpserver"
)

// StreamMedia names the test stream media type and proxy headers.
type StreamMedia string

const (
	MediaTypeNDJSON   StreamMedia = "application/x-ndjson"
	HeaderAccelBuffer StreamMedia = "X-Accel-Buffering"
	AccelBufferingOff StreamMedia = "no"
)

// NDJSONSink streams step events as JSON lines, flushing each; the 200 header is sent on the first line.
type NDJSONSink struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	ctl     *http.ResponseController
	started bool
}

// NewNDJSONSink wraps a response writer.
func NewNDJSONSink(w http.ResponseWriter) *NDJSONSink {
	return &NDJSONSink{w: w, ctl: http.NewResponseController(w)}
}

// Emit implements connections.TestSink.
func (s *NDJSONSink) Emit(e connections.StepEvent) { s.write(e) }

// Finish implements connections.TestSink.
func (s *NDJSONSink) Finish(r connections.TestResult) { s.write(r) }

// Started reports whether any line was written (errors after that cannot become problems).
func (s *NDJSONSink) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func (s *NDJSONSink) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.w.Header().Set(string(httpserver.HeaderContentType), string(MediaTypeNDJSON))
		s.w.Header().Set(string(HeaderAccelBuffer), string(AccelBufferingOff))
		s.w.WriteHeader(http.StatusOK)
		s.started = true
	}
	if err := json.NewEncoder(s.w).Encode(v); err != nil {
		return
	}
	_ = s.ctl.Flush()
}

// TestStream executes a prepared test into an NDJSON response.
type TestStream struct{}

// Run streams the run; errors before the first line are returned as problems, later ones end the stream.
func (TestStream) Run(w http.ResponseWriter, r *http.Request, tester *connections.Tester, p connections.PreparedTest, opts connections.TestOptions) error {
	sink := NewNDJSONSink(w)
	err := tester.Execute(r.Context(), p, opts, sink)
	if err != nil && !sink.Started() {
		return err
	}
	return nil
}

// TestSavedHandler serves POST /api/v1/connections/{id}/test.
type TestSavedHandler struct{ d Deps }

// Route implements api.Handler.
func (h *TestSavedHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTestSaved), Permission: PermWrite, Object: h.d.Object, Mutating: true, AuditAction: connections.ActionTested}
}

// Serve implements api.Handler.
func (h *TestSavedHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	in, err := api.Decoder[TestOptionsInput]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	prepared, err := h.d.Tester.PrepareSaved(r.Context(), id)
	if err != nil {
		return err
	}
	return TestStream{}.Run(w, r, h.d.Tester, prepared, connections.TestOptions{ReadTopic: in.ReadTopic, WriteTopic: in.WriteTopic})
}

// TestDraftHandler serves POST /api/v1/connections/test (unsaved draft; no health update).
type TestDraftHandler struct{ d Deps }

// Route implements api.Handler.
func (h *TestDraftHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTestDraft), Permission: PermWrite, Mutating: true, AuditAction: connections.ActionTested}
}

// Serve implements api.Handler.
func (h *TestDraftHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[DraftTestInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	team, err := DraftTeam{d: h.d}.Resolve(r.Context(), in)
	if err != nil {
		return err
	}
	prepared, err := h.d.Tester.PrepareDraft(r.Context(), in.Draft(team), in.ConnectionID)
	if err != nil {
		return err
	}
	return TestStream{}.Run(w, r, h.d.Tester, prepared, connections.TestOptions{ReadTopic: in.ReadTopic, WriteTopic: in.WriteTopic})
}
