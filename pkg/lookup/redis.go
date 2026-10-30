package lookup

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// RedisOp names the supported commands (document `request.op`).
type RedisOp string

const (
	RedisGet  RedisOp = "get"
	RedisHGet RedisOp = "hget"
	RedisMGet RedisOp = "mget"
)

// RedisTransientReplies are server reply prefixes that heal on retry.
var RedisTransientReplies = []string{"LOADING", "BUSY", "TRYAGAIN", "CLUSTERDOWN", "MASTERDOWN"}

// RedisAuthReplies are server reply prefixes of failed authentication.
var RedisAuthReplies = []string{"NOAUTH", "WRONGPASS", "NOPERM"}

// ErrRedisOp is returned for an unsupported op.
var ErrRedisOp = errors.New("redis op is not supported")

// RedisCommand runs one op and returns the raw answer (nil = miss).
type RedisCommand interface {
	Op() RedisOp
	Run(ctx context.Context, c redis.Cmdable, req ctxhelpers.SourceRequest) (any, bool, error)
}

// RedisGetCommand is GET key.
type RedisGetCommand struct{}

func (RedisGetCommand) Op() RedisOp { return RedisGet }
func (RedisGetCommand) Run(ctx context.Context, c redis.Cmdable, req ctxhelpers.SourceRequest) (any, bool, error) {
	return RedisReplies{}.Scalar(c.Get(ctx, req.Key).Result())
}

// RedisHGetCommand is HGET key field.
type RedisHGetCommand struct{}

func (RedisHGetCommand) Op() RedisOp { return RedisHGet }
func (RedisHGetCommand) Run(ctx context.Context, c redis.Cmdable, req ctxhelpers.SourceRequest) (any, bool, error) {
	return RedisReplies{}.Scalar(c.HGet(ctx, req.Key, req.HashField).Result())
}

// RedisMGetCommand is MGET keys…; the answer is an array, a miss when every key is absent.
type RedisMGetCommand struct{}

func (RedisMGetCommand) Op() RedisOp { return RedisMGet }
func (RedisMGetCommand) Run(ctx context.Context, c redis.Cmdable, req ctxhelpers.SourceRequest) (any, bool, error) {
	values, err := c.MGet(ctx, req.Keys...).Result()
	if err != nil {
		return nil, false, err
	}
	out, found := make([]any, len(values)), false
	for i, v := range values {
		if s, ok := v.(string); ok {
			out[i], found = AnswerDecoder{}.JSONOrText([]byte(s)), true
		}
	}
	return out, found, nil
}

// RedisCommands selects the command of an op.
var RedisCommands = map[RedisOp]RedisCommand{RedisGet: RedisGetCommand{}, RedisHGet: RedisHGetCommand{}, RedisMGet: RedisMGetCommand{}}

// RedisReplies converts scalar replies.
type RedisReplies struct{}

// Scalar maps redis.Nil to a miss and decodes JSON values.
func (RedisReplies) Scalar(s string, err error) (any, bool, error) {
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return AnswerDecoder{}.JSONOrText([]byte(s)), true, nil
}

// RedisFactory builds Redis clients.
type RedisFactory struct{}

func (RedisFactory) Kind() SourceKind { return KindRedis }

func (RedisFactory) New(spec SourceSpec) (Client, error) {
	if spec.Redis == nil {
		return nil, fmt.Errorf("redis settings missing")
	}
	opts := &redis.Options{Addr: spec.Redis.Addr, DB: spec.Redis.DB, Username: spec.Redis.Username, Password: spec.Secrets.Password,
		PoolSize: spec.Limits.MaxConcurrency, MaxRetries: -1}
	if spec.Redis.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &RedisClient{client: redis.NewClient(opts)}, nil
}

// RedisClient calls a Redis source.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClientWith wraps an existing client (tests with miniredis).
func NewRedisClientWith(c *redis.Client) *RedisClient { return &RedisClient{client: c} }

// Fetch implements Client.
func (c *RedisClient) Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	cmd, ok := RedisCommands[RedisOp(req.Op)]
	if !ok {
		return ctxhelpers.SourceAnswer{}, Failures{}.Permanent(ReasonRejected, fmt.Errorf("%w: %q", ErrRedisOp, req.Op))
	}
	v, found, err := cmd.Run(ctx, c.client, req)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, RedisErrors{}.Classify(err)
	}
	return ctxhelpers.SourceAnswer{Found: found, Value: v, Mode: ctxhelpers.AnswerLive}, nil
}

// Probe runs PING: auth replies fail authenticated, other errors fail reachable.
func (c *RedisClient) Probe(ctx context.Context) error {
	err := c.client.Ping(ctx).Err()
	if err == nil {
		return nil
	}
	if (RedisErrors{}).auth(err) {
		return &ProbeError{Stage: StageAuthenticated, Hint: string(HintCredentials), Cause: err}
	}
	return &ProbeError{Stage: StageReachable, Hint: string(HintReachable), Cause: err}
}

// Close implements Client.
func (c *RedisClient) Close() { _ = c.client.Close() }

// RedisErrors classifies Redis failures.
type RedisErrors struct{}

// Classify returns a transient error for network/deadline/retryable replies, else permanent.
func (e RedisErrors) Classify(err error) error {
	var reply redis.Error
	if errors.As(err, &reply) {
		if e.hasPrefix(reply.Error(), RedisTransientReplies) {
			return Failures{}.Transient(ReasonUnavailable, err)
		}
		return Failures{}.Permanent(ReasonRejected, err)
	}
	return Failures{}.Transient(ReasonUnavailable, err)
}

func (e RedisErrors) auth(err error) bool { return e.hasPrefix(err.Error(), RedisAuthReplies) }

func (RedisErrors) hasPrefix(msg string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}
