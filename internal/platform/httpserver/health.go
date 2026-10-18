package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// ReadinessCheck reports whether one dependency (DB, Redis, ...) is usable.
type ReadinessCheck interface {
	Name() string
	Check(ctx context.Context) error
}

// ProbeInterval is how often readiness checks run in the background.
type ProbeInterval time.Duration

const (
	DefaultProbeInterval ProbeInterval = ProbeInterval(5 * time.Second)
	DefaultProbeTimeout  ProbeInterval = ProbeInterval(2 * time.Second)
)

// PendingReason is reported for checks that have not run yet.
type PendingReason string

const ReasonPending PendingReason = "not checked yet"

// Health serves liveness and readiness probes; readiness uses the cached result of background checks.
type Health struct {
	ready    atomic.Bool
	checks   []ReadinessCheck
	failures atomic.Pointer[map[string]string]
}

// NewHealth creates a Health that is not ready until MarkReady is called and checks have passed.
func NewHealth(checks ...ReadinessCheck) *Health {
	h := &Health{checks: checks}
	pending := map[string]string{}
	for _, c := range checks {
		pending[c.Name()] = string(ReasonPending)
	}
	h.failures.Store(&pending)
	return h
}

// AddCheck registers another readiness check before Watch starts.
func (h *Health) AddCheck(c ReadinessCheck) {
	h.checks = append(h.checks, c)
	failures := *h.failures.Load()
	failures[c.Name()] = string(ReasonPending)
}

// MarkReady flips the readiness state (false during startup and shutdown).
func (h *Health) MarkReady(ready bool) {
	h.ready.Store(ready)
}

// Watch runs all checks immediately and then every interval until ctx ends.
func (h *Health) Watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		h.runChecks(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Liveness answers 200 while the process is able to serve HTTP.
func (h *Health) Liveness(w http.ResponseWriter, _ *http.Request) {
	h.respond(w, http.StatusOK, map[string]ProbeStatus{"status": ProbeOK})
}

// Readiness answers 200 only when the server is up and the last checks passed.
func (h *Health) Readiness(w http.ResponseWriter, _ *http.Request) {
	failures := *h.failures.Load()
	if !h.ready.Load() || len(failures) > 0 {
		h.respond(w, http.StatusServiceUnavailable, map[string]any{"status": ProbeNotReady, "failures": failures})
		return
	}
	h.respond(w, http.StatusOK, map[string]ProbeStatus{"status": ProbeOK})
}

func (h *Health) runChecks(ctx context.Context) {
	failures := map[string]string{}
	for _, c := range h.checks {
		checkCtx, cancel := context.WithTimeout(ctx, time.Duration(DefaultProbeTimeout))
		if err := c.Check(checkCtx); err != nil {
			failures[c.Name()] = err.Error()
		}
		cancel()
	}
	h.failures.Store(&failures)
}

func (h *Health) respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set(string(HeaderContentType), string(MediaTypeJSON))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
