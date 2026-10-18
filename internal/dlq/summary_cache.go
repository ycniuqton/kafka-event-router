package dlq

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Summary is the unresolved-count summary of a route (design §4.2).
type Summary struct {
	Unresolved  int                      `json:"unresolved"`
	ByErrorType map[record.ErrorType]int `json:"byErrorType"`
	OldestAt    *time.Time               `json:"oldestAt"`
	NewestAt    *time.Time               `json:"newestAt"`
	Approximate bool                     `json:"approximate"`
	Warnings    []Warning                `json:"warnings,omitempty"`
}

// SummaryCache caches route summaries for a short TTL.
type SummaryCache interface {
	Get(ctx context.Context, routeID string) (Summary, bool)
	Set(ctx context.Context, routeID string, s Summary)
	Invalidate(ctx context.Context, routeID string)
}

// SummaryKeyPrefix is the Redis key prefix of cached summaries.
type SummaryKeyPrefix string

const summaryPrefix SummaryKeyPrefix = "dlq:summary:"

// RedisSummaryCache stores summaries under dlq:summary:{route} (TTL 15 s).
type RedisSummaryCache struct {
	Client redis.UniversalClient
}

// Get implements SummaryCache.
func (c RedisSummaryCache) Get(ctx context.Context, routeID string) (Summary, bool) {
	raw, err := c.Client.Get(ctx, string(summaryPrefix)+routeID).Bytes()
	if err != nil {
		return Summary{}, false
	}
	var s Summary
	return s, json.Unmarshal(raw, &s) == nil
}

// Set implements SummaryCache.
func (c RedisSummaryCache) Set(ctx context.Context, routeID string, s Summary) {
	raw, _ := json.Marshal(s)
	c.Client.Set(ctx, string(summaryPrefix)+routeID, raw, SummaryTTL.Duration())
}

// Invalidate implements SummaryCache.
func (c RedisSummaryCache) Invalidate(ctx context.Context, routeID string) {
	c.Client.Del(context.WithoutCancel(ctx), string(summaryPrefix)+routeID)
}

// NoSummaryCache never caches.
type NoSummaryCache struct{}

func (NoSummaryCache) Get(context.Context, string) (Summary, bool) { return Summary{}, false }
func (NoSummaryCache) Set(context.Context, string, Summary)        {}
func (NoSummaryCache) Invalidate(context.Context, string)          {}
