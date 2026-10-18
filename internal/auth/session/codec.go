package session

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// fields encodes a session as Redis hash fields.
func (s *RedisStore) fields(sess auth.Session) map[string]any {
	return map[string]any{
		string(FieldUserID):    sess.UserID.String(),
		string(FieldMode):      string(sess.Mode),
		string(FieldCSRF):      sess.CSRFToken,
		string(FieldIP):        sess.IP,
		string(FieldUserAgent): sess.UserAgent,
		string(FieldCreatedAt): sess.CreatedAt.UTC().Format(time.RFC3339Nano),
		string(FieldLastSeen):  sess.LastSeenAt.UTC().Format(time.RFC3339Nano),
	}
}

// decode rebuilds a session from Redis hash fields; corrupt data counts as not found.
func (s *RedisStore) decode(id string, v map[string]string) (auth.Session, error) {
	userID, err := uuid.Parse(v[string(FieldUserID)])
	if err != nil {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	created, err := time.Parse(time.RFC3339Nano, v[string(FieldCreatedAt)])
	if err != nil {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	lastSeen, err := time.Parse(time.RFC3339Nano, v[string(FieldLastSeen)])
	if err != nil {
		lastSeen = created
	}
	return auth.Session{
		ID: id, UserID: userID, Mode: auth.AuthMode(v[string(FieldMode)]), CSRFToken: v[string(FieldCSRF)],
		IP: v[string(FieldIP)], UserAgent: v[string(FieldUserAgent)], CreatedAt: created, LastSeenAt: lastSeen,
	}, nil
}
