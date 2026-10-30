package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

// SessionConnector opens one dedicated pgx connection (SessionDialer in production).
type SessionConnector interface {
	Connect(ctx context.Context) (*pgx.Conn, error)
}

// HealthProbe pings Postgres on its own lazily dialled session, redialling after a failure, so readiness
// never waits for a pooled connection.
type HealthProbe struct {
	dialer SessionConnector
	mu     sync.Mutex
	conn   *pgx.Conn
}

// NewHealthProbe builds a probe; nothing is dialled before the first Check.
func NewHealthProbe(dialer SessionConnector) *HealthProbe {
	return &HealthProbe{dialer: dialer}
}

// Check pings the session, dialling it first when needed; a failed session is dropped.
func (p *HealthProbe) Check(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		conn, err := p.dialer.Connect(ctx)
		if err != nil {
			return err
		}
		p.conn = conn
	}
	if err := p.conn.Ping(ctx); err != nil {
		_ = p.conn.Close(context.Background())
		p.conn = nil
		return err
	}
	return nil
}

// Close ends the session.
func (p *HealthProbe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close(context.Background())
		p.conn = nil
	}
}
