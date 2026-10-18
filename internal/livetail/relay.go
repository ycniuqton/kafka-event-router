package livetail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
)

// ClientOpKind names the messages a tail client sends.
type ClientOpKind string

const (
	OpPause  ClientOpKind = "pause"
	OpResume ClientOpKind = "resume"
	OpFilter ClientOpKind = "filter"
)

// ClientOp is one client message: `{op:"pause"|"resume"|"filter", outcomes?, text?}`.
type ClientOp struct {
	Op       ClientOpKind  `json:"op"`
	Outcomes []TailOutcome `json:"outcomes,omitempty"`
	Text     string        `json:"text,omitempty"`
}

// FrameType names the frames the server sends.
type FrameType string

const (
	FrameRecord FrameType = "record"
	FrameStatus FrameType = "status"
)

// StatusState is the stream state a status frame reports.
type StatusState string

const (
	StateLive      StatusState = "live"
	StatePaused    StatusState = "paused"
	StateThrottled StatusState = "throttled"
)

// Frame is one server message: a record (with its masked flag) or a status.
type Frame struct {
	Type   FrameType   `json:"type"`
	Record *TailRecord `json:"record,omitempty"`
	Masked bool        `json:"masked"`
	State  StatusState `json:"state,omitempty"`
}

// TailFilter keeps records whose outcome is selected (none selected = all) and whose JSON contains Text.
type TailFilter struct {
	Outcomes map[TailOutcome]bool
	Text     string
}

// FilterOf builds the filter of a client op; unknown outcomes are ignored.
func FilterOf(op ClientOp) TailFilter {
	f := TailFilter{Outcomes: map[TailOutcome]bool{}, Text: strings.ToLower(strings.TrimSpace(op.Text))}
	for _, o := range op.Outcomes {
		if o.Valid() {
			f.Outcomes[o] = true
		}
	}
	return f
}

// Match reports whether the (already masked) record passes; rendered is its JSON.
func (f TailFilter) Match(rec TailRecord, rendered []byte) bool {
	if len(f.Outcomes) > 0 && !f.Outcomes[rec.Outcome] {
		return false
	}
	return f.Text == "" || bytes.Contains(bytes.ToLower(rendered), []byte(f.Text))
}

// FrameConn is the socket a session writes frames to and reads client ops from.
type FrameConn interface {
	WriteJSON(ctx context.Context, v any) error
	ReadJSON(ctx context.Context, v any) error
}

// ErrTailUnavailable means the tail sink (Redis) cannot be used; the socket closes with 1011.
var ErrTailUnavailable = errors.New("live tail unavailable")

// SessionLog names the session's log messages.
type SessionLog string

const LogRenewFailed SessionLog = "livetail: flag renew failed"

// TailSession relays one route's tail to one socket: it keeps the watch flag alive, reads new stream
// entries, masks and filters them, and honours pause/resume/filter ops (paused records are discarded).
type TailSession struct {
	RouteID string
	Store   TailStore
	Masker  TailMasker
	Masked  bool
	Limits  TailLimits
	Log     *slog.Logger

	filter TailFilter
	paused bool
}

// sessionChannels carries what the background readers produce.
type sessionChannels struct {
	entries chan []StreamEntry
	ops     chan ClientOp
	failed  chan error
}

// Run streams until ctx ends or the client goes away (nil), or the store fails (ErrTailUnavailable).
func (s *TailSession) Run(ctx context.Context, conn FrameConn) error {
	if err := s.Store.Watch(ctx, s.RouteID); err != nil {
		return ErrTailUnavailable
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := sessionChannels{entries: make(chan []StreamEntry), ops: make(chan ClientOp), failed: make(chan error, 1)}
	go s.renew(ctx)
	go s.readStream(ctx, ch)
	go s.readOps(ctx, cancel, conn, ch)
	if err := s.status(ctx, conn, StateLive); err != nil {
		return nil
	}
	return s.loop(ctx, conn, ch)
}

// loop is the single writer of the socket.
func (s *TailSession) loop(ctx context.Context, conn FrameConn, ch sessionChannels) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ch.failed:
			return ErrTailUnavailable
		case op := <-ch.ops:
			if err := s.apply(ctx, conn, op); err != nil {
				return nil
			}
		case batch := <-ch.entries:
			if err := s.deliver(ctx, conn, batch); err != nil {
				return nil
			}
		}
	}
}

// apply handles one client op and confirms the resulting state.
func (s *TailSession) apply(ctx context.Context, conn FrameConn, op ClientOp) error {
	switch op.Op {
	case OpPause:
		s.paused = true
	case OpResume:
		s.paused = false
	case OpFilter:
		s.filter = FilterOf(op)
		return nil
	default:
		return nil
	}
	return s.status(ctx, conn, s.state())
}

// state is live or paused.
func (s *TailSession) state() StatusState {
	if s.paused {
		return StatePaused
	}
	return StateLive
}

// deliver sends the batch's records that pass the filter; throttle notices become status frames.
func (s *TailSession) deliver(ctx context.Context, conn FrameConn, batch []StreamEntry) error {
	if s.paused {
		return nil
	}
	for _, e := range batch {
		if err := s.deliverOne(ctx, conn, e); err != nil {
			return err
		}
	}
	return nil
}

// deliverOne masks, filters and writes one entry.
func (s *TailSession) deliverOne(ctx context.Context, conn FrameConn, e StreamEntry) error {
	if e.Kind == EntryThrottled {
		return s.status(ctx, conn, StateThrottled)
	}
	var rec TailRecord
	if json.Unmarshal(e.Data, &rec) != nil {
		return nil
	}
	if s.Masked {
		rec = s.Masker.Mask(rec)
	}
	rendered, err := json.Marshal(rec)
	if err != nil || !s.filter.Match(rec, rendered) {
		return nil
	}
	return conn.WriteJSON(ctx, Frame{Type: FrameRecord, Record: &rec, Masked: s.Masked})
}

func (s *TailSession) status(ctx context.Context, conn FrameConn, st StatusState) error {
	return conn.WriteJSON(ctx, Frame{Type: FrameStatus, State: st, Masked: s.Masked})
}

// renew keeps the flag alive every RenewEvery while the socket is open.
func (s *TailSession) renew(ctx context.Context) {
	t := time.NewTicker(s.Limits.RenewEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Store.Watch(ctx, s.RouteID); err != nil && ctx.Err() == nil {
				s.Log.Warn(string(LogRenewFailed), string(AttrRoute), s.RouteID, string(logging.AttrError), err)
			}
		}
	}
}

// readStream blocks on the stream and hands non-empty batches to the loop; a store error ends the session.
func (s *TailSession) readStream(ctx context.Context, ch sessionChannels) {
	after := string(StreamFromNow)
	for ctx.Err() == nil {
		batch, err := s.Store.Read(ctx, s.RouteID, after)
		if err != nil {
			if ctx.Err() == nil {
				ch.failed <- err
			}
			return
		}
		if len(batch) == 0 {
			continue
		}
		after = batch[len(batch)-1].ID
		select {
		case ch.entries <- batch:
		case <-ctx.Done():
			return
		}
	}
}

// readOps decodes client messages; a read error (client gone) ends the session.
func (s *TailSession) readOps(ctx context.Context, cancel context.CancelFunc, conn FrameConn, ch sessionChannels) {
	defer cancel()
	for {
		var op ClientOp
		if err := conn.ReadJSON(ctx, &op); err != nil {
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) {
				continue
			}
			return
		}
		select {
		case ch.ops <- op:
		case <-ctx.Done():
			return
		}
	}
}
