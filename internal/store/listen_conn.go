package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification is one NOTIFY received on a listener session.
type Notification struct {
	Channel Channel
	Payload string
}

// ListenConn is the base of one dedicated LISTEN session (a real pgx connection or a test fake).
type ListenConn interface {
	Listen(ctx context.Context, ch Channel) error
	WaitForNotification(ctx context.Context) (Notification, error)
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}

// ListenDialer opens dedicated listener sessions.
type ListenDialer interface {
	Dial(ctx context.Context) (ListenConn, error)
}

// SessionDialer opens dedicated pgx connections outside the pool, from the pool's connection settings.
type SessionDialer struct {
	Config *pgx.ConnConfig
}

// SessionDialerOf derives a SessionDialer from pool (same DSN, credentials and TLS).
func SessionDialerOf(pool *pgxpool.Pool) SessionDialer {
	return SessionDialer{Config: pool.Config().ConnConfig}
}

// Connect dials a new dedicated connection; the caller closes it.
func (d SessionDialer) Connect(ctx context.Context) (*pgx.Conn, error) {
	return pgx.ConnectConfig(ctx, d.Config.Copy())
}

// Dial implements ListenDialer.
func (d SessionDialer) Dial(ctx context.Context) (ListenConn, error) {
	conn, err := d.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return PgListenConn{conn: conn}, nil
}

// PgListenConn is a ListenConn over a dedicated pgx connection.
type PgListenConn struct {
	conn *pgx.Conn
}

// Listen implements ListenConn (the channel name is quoted as an identifier).
func (c PgListenConn) Listen(ctx context.Context, ch Channel) error {
	_, err := c.conn.Exec(ctx, "LISTEN "+pgx.Identifier{string(ch)}.Sanitize())
	return err
}

// WaitForNotification implements ListenConn; a cancelled ctx leaves the session usable.
func (c PgListenConn) WaitForNotification(ctx context.Context) (Notification, error) {
	n, err := c.conn.WaitForNotification(ctx)
	if err != nil {
		return Notification{}, err
	}
	return Notification{Channel: Channel(n.Channel), Payload: n.Payload}, nil
}

// Ping implements ListenConn.
func (c PgListenConn) Ping(ctx context.Context) error { return c.conn.Ping(ctx) }

// Close implements ListenConn.
func (c PgListenConn) Close(ctx context.Context) error { return c.conn.Close(ctx) }
