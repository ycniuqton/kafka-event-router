package preview

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/pkg/config"
)

// RedisKey names the Redis key layout of sample sessions.
type RedisKey string

const (
	RedisSessionPrefix RedisKey = "preview:session:"
	RedisRecordsSuffix RedisKey = ":records"
	RedisSchemaSuffix  RedisKey = ":schema"
)

// RedisSessionStore keeps session JSON, a record list and the schema under one TTL.
type RedisSessionStore struct {
	client redis.UniversalClient
}

// NewRedisSessionStore wraps a Redis client.
func NewRedisSessionStore(client redis.UniversalClient) *RedisSessionStore {
	return &RedisSessionStore{client: client}
}

func (s *RedisSessionStore) Kind() StoreKind { return StoreRedis }

// RedisKeys builds the keys of one session.
type RedisKeys struct{ id SessionID }

func (k RedisKeys) Meta() string    { return string(RedisSessionPrefix) + string(k.id) }
func (k RedisKeys) Records() string { return k.Meta() + string(RedisRecordsSuffix) }
func (k RedisKeys) Schema() string  { return k.Meta() + string(RedisSchemaSuffix) }

// Create writes the session and its records atomically with the TTL.
func (s *RedisSessionStore) Create(ctx context.Context, sess Session, recs []SampleRecord, ttl time.Duration) error {
	meta, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	items, err := s.encodeRecords(recs)
	if err != nil {
		return err
	}
	keys := RedisKeys{id: sess.ID}
	_, err = s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, keys.Records(), keys.Schema())
		if len(items) > 0 {
			p.RPush(ctx, keys.Records(), items...)
			p.Expire(ctx, keys.Records(), ttl)
		}
		p.Set(ctx, keys.Meta(), meta, ttl)
		return nil
	})
	return err
}

func (s *RedisSessionStore) encodeRecords(recs []SampleRecord) ([]any, error) {
	items := make([]any, 0, len(recs))
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, nil
}

// Get returns the session or ErrSessionExpired.
func (s *RedisSessionStore) Get(ctx context.Context, id SessionID) (Session, error) {
	raw, err := s.client.Get(ctx, RedisKeys{id: id}.Meta()).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrSessionExpired
	}
	if err != nil {
		return Session{}, err
	}
	var sess Session
	return sess, json.Unmarshal(raw, &sess)
}

// Records returns count records starting at start (session must exist).
func (s *RedisSessionStore) Records(ctx context.Context, id SessionID, start, count int) ([]SampleRecord, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if count <= 0 {
		return []SampleRecord{}, nil
	}
	items, err := s.client.LRange(ctx, RedisKeys{id: id}.Records(), int64(start), int64(start+count-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]SampleRecord, 0, len(items))
	for _, it := range items {
		var r SampleRecord
		if err := json.Unmarshal([]byte(it), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// All returns every record.
func (s *RedisSessionStore) All(ctx context.Context, id SessionID) ([]SampleRecord, error) {
	return s.Records(ctx, id, 0, int(MaxEvents))
}

// PutSchema caches the schema with the session's remaining TTL.
func (s *RedisSessionStore) PutSchema(ctx context.Context, id SessionID, fs FieldSchema) error {
	keys := RedisKeys{id: id}
	ttl, err := s.client.TTL(ctx, keys.Meta()).Result()
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return ErrSessionExpired
	}
	b, err := json.Marshal(fs)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, keys.Schema(), b, ttl).Err()
}

// Schema returns the cached schema, if any.
func (s *RedisSessionStore) Schema(ctx context.Context, id SessionID) (FieldSchema, bool, error) {
	raw, err := s.client.Get(ctx, RedisKeys{id: id}.Schema()).Bytes()
	if errors.Is(err, redis.Nil) {
		return FieldSchema{}, false, nil
	}
	if err != nil {
		return FieldSchema{}, false, err
	}
	var fs FieldSchema
	return fs, true, json.Unmarshal(raw, &fs)
}

// Touch rewrites the expiry in the session JSON and slides every key's TTL.
func (s *RedisSessionStore) Touch(ctx context.Context, id SessionID, expiresAt time.Time, ttl time.Duration) error {
	sess, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	sess.ExpiresAt = expiresAt
	meta, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	keys := RedisKeys{id: id}
	_, err = s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, keys.Meta(), meta, ttl)
		p.Expire(ctx, keys.Records(), ttl)
		p.Expire(ctx, keys.Schema(), ttl)
		return nil
	})
	return err
}

// Reformat rewrites the session format and expiry, drops the cached schema and slides the TTL.
func (s *RedisSessionStore) Reformat(ctx context.Context, id SessionID, format config.FormatType, expiresAt time.Time, ttl time.Duration) error {
	sess, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	sess.Format, sess.ExpiresAt = format, expiresAt
	meta, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	keys := RedisKeys{id: id}
	_, err = s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, keys.Meta(), meta, ttl)
		p.Expire(ctx, keys.Records(), ttl)
		p.Del(ctx, keys.Schema())
		return nil
	})
	return err
}

// Delete removes every key of the session.
func (s *RedisSessionStore) Delete(ctx context.Context, id SessionID) error {
	keys := RedisKeys{id: id}
	return s.client.Del(ctx, keys.Meta(), keys.Records(), keys.Schema()).Err()
}
