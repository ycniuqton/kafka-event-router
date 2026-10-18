package preview

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/pkg/kafka"
)

// ServiceDeps are the collaborators of the preview Service.
type ServiceDeps struct {
	Sources    *SampleSourceManager
	Sessions   *SessionStoreManager
	Runner     PreviewRunner
	Views      SampleViewBuilder
	Schemas    SchemaBuilder
	Masks      MaskPolicy
	TestTopics TestTopicPort
	Metrics    *Metrics
	Log        *slog.Logger
	Now        func() time.Time
	SchemaJobs int
}

// Service is the preview use-case entry point behind /internal/v1/preview (one method per endpoint).
type Service struct {
	d    ServiceDeps
	jobs chan struct{}
}

// NewService builds the service with a bounded schema-inference worker budget.
func NewService(d ServiceDeps) *Service {
	return &Service{d: d, jobs: make(chan struct{}, max(d.SchemaJobs, 1))}
}

// Load samples events into a new session and returns its first page.
func (s *Service) Load(ctx context.Context, caller Caller, req LoadRequest) (LoadResponseDTO, error) {
	src, err := s.d.Sources.For(req.Mode)
	if err != nil {
		return LoadResponseDTO{}, err
	}
	if !s.d.Views.Decoder.Supports(req.EffectiveFormat()) {
		return LoadResponseDTO{}, ErrUnknownFormat.At(string(FieldFormat))
	}
	start := s.d.Now()
	lctx, cancel := context.WithTimeout(ctx, LoadDeadline.Duration())
	defer cancel()
	budget := kafka.NewSamplingBudget(int(MaxEvents), int64(MaxBytes), start.Add(LoadDeadline.Duration()))
	res, err := src.Load(lctx, req, budget)
	if err != nil {
		return LoadResponseDTO{}, err
	}
	sess, err := s.d.Sessions.Create(ctx, SessionFactory{}.New(caller, req, res), res.Records)
	if err != nil {
		return LoadResponseDTO{}, err
	}
	s.d.Metrics.Load(req.Mode, s.d.Now().Sub(start))
	s.d.Metrics.SessionOpened()
	s.inferAsync(sess, res.Records)
	page, next := PageSlicer{}.Slice(res.Records, 0, Page{}.EffectiveLimit())
	return LoadResponseDTO{Session: SessionFactory{}.DTO(sess, caller), Events: s.views(ctx, caller, sess, page), NextCursor: next}, nil
}

// Events returns one page of the session's events, optionally filtered by text.
func (s *Service) Events(ctx context.Context, caller Caller, id SessionID, page Page) (SamplesPageDTO, error) {
	sess, err := s.d.Sessions.Owned(ctx, id, caller)
	if err != nil {
		return SamplesPageDTO{}, err
	}
	start, err := PageSlicer{}.Start(page.Cursor)
	if err != nil {
		return SamplesPageDTO{}, err
	}
	recs, err := s.d.Sessions.All(ctx, id)
	if err != nil {
		return SamplesPageDTO{}, err
	}
	matched := SampleSearcher{}.Filter(recs, page.Query)
	items, next := PageSlicer{}.Slice(matched, start, page.EffectiveLimit())
	if _, err := s.d.Sessions.Touch(ctx, sess); err != nil {
		return SamplesPageDTO{}, err
	}
	return SamplesPageDTO{Events: s.views(ctx, caller, sess, items), NextCursor: next}, nil
}

// views renders records, masked when the caller or the session requires it.
func (s *Service) views(ctx context.Context, caller Caller, sess Session, recs []SampleRecord) []SampleDTO {
	dtos := s.d.Views.DTOs(ctx, recs, sess.SampleFormat())
	if masker, masked := s.d.Masks.For(caller, sess); masked {
		return masker.Samples(dtos)
	}
	return dtos
}

// maskSchema masks schema top values for masked callers or sessions.
func (s *Service) maskSchema(caller Caller, sess Session, fs FieldSchema) FieldSchema {
	if masker, masked := s.d.Masks.For(caller, sess); masked {
		return masker.Schema(fs)
	}
	return fs
}

// Schema returns the cached schema, inferring it now when the async job has not finished.
func (s *Service) Schema(ctx context.Context, caller Caller, id SessionID) (FieldSchema, error) {
	sess, err := s.d.Sessions.Owned(ctx, id, caller)
	if err != nil {
		return FieldSchema{}, err
	}
	if _, err := s.d.Sessions.Touch(ctx, sess); err != nil {
		return FieldSchema{}, err
	}
	if fs, ok, err := s.d.Sessions.Schema(ctx, id); err != nil || (ok && fs.Format == sess.Format) {
		return s.maskSchema(caller, sess, fs), err
	}
	recs, err := s.d.Sessions.All(ctx, id)
	if err != nil {
		return FieldSchema{}, err
	}
	fs := s.d.Schemas.Infer(ctx, recs, sess.SampleFormat())
	return s.maskSchema(caller, sess, fs), s.d.Sessions.PutSchema(ctx, id, fs)
}

// Reformat overrides the session's value format (FR-8) and returns the re-decoded first page.
func (s *Service) Reformat(ctx context.Context, caller Caller, id SessionID, req FormatRequest) (LoadResponseDTO, error) {
	sess, err := s.d.Sessions.Owned(ctx, id, caller)
	if err != nil {
		return LoadResponseDTO{}, err
	}
	if !s.d.Views.Decoder.Supports(req.Format) {
		return LoadResponseDTO{}, ErrUnknownFormat.At(string(FieldFormat))
	}
	if sess, err = s.d.Sessions.Reformat(ctx, sess, req.Format); err != nil {
		return LoadResponseDTO{}, err
	}
	recs, err := s.d.Sessions.All(ctx, id)
	if err != nil {
		return LoadResponseDTO{}, err
	}
	s.inferAsync(sess, recs)
	page, next := PageSlicer{}.Slice(recs, 0, Page{}.EffectiveLimit())
	return LoadResponseDTO{Session: SessionFactory{}.DTO(sess, caller), Events: s.views(ctx, caller, sess, page), NextCursor: next}, nil
}

// Source returns where the caller's session samples come from.
func (s *Service) Source(ctx context.Context, caller Caller, id SessionID) (SessionSourceDTO, error) {
	sess, err := s.d.Sessions.Owned(ctx, id, caller)
	if err != nil {
		return SessionSourceDTO{}, err
	}
	return SessionSourceDTO{ID: sess.ID, ConnectionID: sess.ConnectionID, Mode: sess.Mode, Masked: sess.Masked}, nil
}

// Delete closes the caller's session.
func (s *Service) Delete(ctx context.Context, caller Caller, id SessionID) error {
	if _, err := s.d.Sessions.Owned(ctx, id, caller); err != nil {
		return err
	}
	s.d.Metrics.SessionClosed()
	return s.d.Sessions.Delete(ctx, id)
}

// Run runs a draft over the session (or inline events) and slides the session TTL.
func (s *Service) Run(ctx context.Context, caller Caller, req RunRequest, mode RunMode) (RunResultDTO, error) {
	start := s.d.Now()
	plan, err := RunInputs{Sessions: s.d.Sessions, Now: s.d.Now}.Plan(ctx, caller, req, mode)
	if err != nil {
		return RunResultDTO{}, err
	}
	if mode == RunTest && req.WriteToTopic {
		plan.Sink = &OutputCollector{}
	}
	res, err := s.d.Runner.Run(ctx, plan)
	s.d.Metrics.Run(RunOutcomes{}.Of(res, err), s.d.Now().Sub(start))
	if err == nil && plan.Sink != nil {
		res.TestTopic, err = s.writeTestTopic(ctx, caller, req, plan.Sink)
	}
	if masker, masked := s.d.Masks.For(caller, plan.Session); masked && err == nil {
		res = masker.Run(res)
	}
	return res, err
}

// writeTestTopic produces the passed outputs of a test run to a temporary test topic (FR-22).
func (s *Service) writeTestTopic(ctx context.Context, caller Caller, req RunRequest, sink *OutputCollector) (*TestTopicDTO, error) {
	if s.d.TestTopics == nil {
		return nil, ErrTestTopicFailed
	}
	return s.d.TestTopics.Write(ctx, TestTopicRequest{Caller: caller, RouteID: req.RouteID, Refs: DraftRefReader{}.Of(req.Config), Records: sink.Records()})
}

// TestTopicPort writes test-run outputs to a test topic (implemented by TestTopicWriter).
type TestTopicPort interface {
	Write(ctx context.Context, req TestTopicRequest) (*TestTopicDTO, error)
}

// inferAsync infers the schema in the background when a worker slot is free.
func (s *Service) inferAsync(sess Session, recs []SampleRecord) {
	select {
	case s.jobs <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-s.jobs }()
		ctx, cancel := context.WithTimeout(context.Background(), LoadDeadline.Duration())
		defer cancel()
		fs := s.d.Schemas.Infer(ctx, recs, sess.SampleFormat())
		if err := s.d.Sessions.PutSchema(ctx, sess.ID, fs); err != nil {
			s.d.Log.Warn(string(LogSchemaFailed), string(logging.AttrError), err)
		}
	}()
}

// LogEvent names preview log events.
type LogEvent string

const (
	LogSchemaFailed            LogEvent = "preview.schema_inference_failed"
	LogGatewayDisabled         LogEvent = "preview.gateway_disabled"
	LogControlPlaneUnavailable LogEvent = "preview.control_plane_unavailable"
)

// SessionFactory builds sessions and their public view.
type SessionFactory struct{}

// New builds the session metadata for a load result.
func (SessionFactory) New(caller Caller, req LoadRequest, res LoadResult) Session {
	topics := req.Topics
	if topics == nil {
		topics = []string{}
	}
	return Session{
		ID: NewSessionID(), OwnerID: caller.UserID, TeamID: caller.TeamID, ConnectionID: req.ConnectionID, Topics: topics,
		Mode: req.Mode, Format: req.EffectiveFormat(), Loaded: len(res.Records), Bytes: res.Bytes,
		Truncated: res.StopReason.Truncated(), StopReason: res.StopReason, Masked: caller.Mask, MaskStyle: caller.MaskStyle,
	}
}

// DTO renders the session for the caller.
func (SessionFactory) DTO(s Session, caller Caller) SessionDTO {
	return SessionDTO{ID: s.ID, ExpiresAt: s.ExpiresAt, Loaded: s.Loaded, Bytes: s.Bytes, Truncated: s.Truncated, StopReason: s.StopReason, Masked: s.Masked || caller.Mask, Format: s.Format}
}

// PageSlicer pages record lists with decimal offset cursors.
type PageSlicer struct{}

// Start decodes a cursor (empty = first page).
func (PageSlicer) Start(c Cursor) (int, error) {
	if c == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(string(c))
	if err != nil || n < 0 {
		return 0, ErrBadCursor
	}
	return n, nil
}

// Slice returns records [start, start+limit) and the next cursor, if any.
func (PageSlicer) Slice(recs []SampleRecord, start, limit int) ([]SampleRecord, Cursor) {
	start = min(start, len(recs))
	end := min(start+limit, len(recs))
	if end >= len(recs) {
		return recs[start:end], ""
	}
	return recs[start:end], Cursor(strconv.Itoa(end))
}

// RunOutcomes classifies runs for metrics.
type RunOutcomes struct{}

// Of returns the outcome label of a run.
func (RunOutcomes) Of(res RunResultDTO, err error) RunOutcome {
	switch {
	case err != nil:
		return RunFailed
	case res.Summary.Incomplete:
		return RunIncomplete
	}
	return RunOK
}
