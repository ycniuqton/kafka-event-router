package httpserver

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/logging"
)

// Server wraps net/http.Server with readiness tracking and graceful shutdown.
// A nil health means the listener does not drive readiness (e.g. the internal listener).
type Server struct {
	cfg    config.HTTPConfig
	health *Health
	log    *slog.Logger
	srv    *http.Server
	tls    *tls.Config
}

// NewServer builds a Server for the given handler; it does not start listening.
func NewServer(cfg config.HTTPConfig, handler http.Handler, health *Health, log *slog.Logger) *Server {
	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: cfg.ReadHeaderTimeout}
	NewDrain().Install(srv)
	return &Server{cfg: cfg, health: health, log: log, srv: srv}
}

// WithTLS serves TLS (and mTLS when the config verifies client certs); nil keeps plain HTTP.
func (s *Server) WithTLS(cfg *tls.Config) *Server {
	s.tls = cfg
	s.srv.TLSConfig = cfg
	return s
}

func (s *Server) markReady(ready bool) {
	if s.health != nil {
		s.health.MarkReady(ready)
	}
}

// Run serves until ctx is cancelled, then drains connections within the shutdown timeout.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	if s.tls != nil {
		ln = tls.NewListener(ln, s.tls)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.Serve(ln) }()
	s.markReady(true)
	s.log.Info("http server listening", string(logging.AttrAddr), ln.Addr().String(), string(logging.AttrTLS), s.tls != nil)

	select {
	case err := <-errCh:
		return s.normalise(err)
	case <-ctx.Done():
		return s.shutdown()
	}
}

func (s *Server) shutdown() error {
	s.markReady(false)
	s.log.Info("http server shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()
	return s.normalise(s.srv.Shutdown(ctx))
}

func (s *Server) normalise(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
