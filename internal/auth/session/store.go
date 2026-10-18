// Package session stores interactive sessions in Redis (never Postgres) and authenticates
// requests carrying the er_session cookie.
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
)

// KeyPrefix names the Redis key prefixes used for sessions.
type KeyPrefix string

const (
	PrefixSession      KeyPrefix = "sess:"
	PrefixUserSessions KeyPrefix = "user_sessions:"
)

// Field names the Redis hash fields of a session.
type Field string

const (
	FieldUserID    Field = "user_id"
	FieldMode      Field = "mode"
	FieldCSRF      Field = "csrf_token"
	FieldIP        Field = "ip"
	FieldUserAgent Field = "user_agent"
	FieldCreatedAt Field = "created_at"
	FieldLastSeen  Field = "last_seen_at"
)

// TokenBytes is the entropy of session ids and CSRF tokens.
type TokenBytes int

const SessionTokenBytes TokenBytes = 32

// TTLs configures idle (sliding) and absolute session lifetimes.
type TTLs struct {
	Idle     time.Duration
	Absolute time.Duration
}

// RedisStore implements auth.SessionStore on a Redis hash per session plus a per-user index set.
type RedisStore struct {
	rdb  redis.UniversalClient
	ttls TTLs
	now  auth.Clock
}

// NewRedisStore builds the store.
func NewRedisStore(rdb redis.UniversalClient, ttls TTLs, now auth.Clock) *RedisStore {
	if now == nil {
		now = time.Now
	}
	return &RedisStore{rdb: rdb, ttls: ttls, now: now}
}

// Create assigns a random id and CSRF token and stores the session with the idle TTL.
func (s *RedisStore) Create(ctx context.Context, sess auth.Session) (auth.Session, error) {
	sess.ID, sess.CSRFToken = s.token(), s.token()
	key := string(PrefixSession) + sess.ID
	userKey := string(PrefixUserSessions) + sess.UserID.String()
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, key, s.fields(sess))
		p.Expire(ctx, key, s.ttlFor(sess))
		p.SAdd(ctx, userKey, sess.ID)
		p.Expire(ctx, userKey, s.ttls.Absolute)
		return nil
	})
	if err != nil {
		return auth.Session{}, auth.ErrAuthUnavailable.Wrap(err)
	}
	return sess, nil
}

// Get loads a session; a missing key yields auth.ErrSessionNotFound.
func (s *RedisStore) Get(ctx context.Context, id string) (auth.Session, error) {
	values, err := s.rdb.HGetAll(ctx, string(PrefixSession)+id).Result()
	if err != nil {
		return auth.Session{}, auth.ErrAuthUnavailable.Wrap(err)
	}
	if len(values) == 0 {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	return s.decode(id, values)
}

// Touch records activity and slides the idle TTL (never past the absolute lifetime).
func (s *RedisStore) Touch(ctx context.Context, sess auth.Session) error {
	sess.LastSeenAt = s.now()
	key := string(PrefixSession) + sess.ID
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, key, string(FieldLastSeen), sess.LastSeenAt.UTC().Format(time.RFC3339Nano))
		p.Expire(ctx, key, s.ttlFor(sess))
		return nil
	})
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	return nil
}

// Revoke deletes one session.
func (s *RedisStore) Revoke(ctx context.Context, id string) error {
	key := string(PrefixSession) + id
	userID, err := s.rdb.HGet(ctx, key, string(FieldUserID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	if userID != "" {
		s.rdb.SRem(ctx, string(PrefixUserSessions)+userID, id)
	}
	return nil
}

// RevokeAllForUser deletes every session of the user (e.g. when disabled).
func (s *RedisStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	userKey := string(PrefixUserSessions) + userID.String()
	ids, err := s.rdb.SMembers(ctx, userKey).Result()
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	keys := []string{userKey}
	for _, id := range ids {
		keys = append(keys, string(PrefixSession)+id)
	}
	if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	return nil
}

func (s *RedisStore) ttlFor(sess auth.Session) time.Duration {
	remaining := s.ttls.Absolute - s.now().Sub(sess.CreatedAt)
	return min(s.ttls.Idle, remaining)
}

func (s *RedisStore) token() string {
	b := make([]byte, SessionTokenBytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
