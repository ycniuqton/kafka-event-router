package retrytopic

import (
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/dlq/record"
)

// TierPrefix is the prefix of retry tier topics.
type TierPrefix string

const DefaultTierPrefix TierPrefix = "router.retry."

// TierNaming derives the tier topics of a route: router.retry.<slug(routeName)>.<n>, n = 1..maxAttempts.
type TierNaming struct{}

// Topic returns the tier topic of attempt n.
func (TierNaming) Topic(routeName string, n int) string {
	slug := strings.TrimPrefix(record.TopicNaming{}.Default(routeName), string(record.DefaultTopicPrefix))
	return string(DefaultTierPrefix) + slug + "." + strconv.Itoa(n)
}

// Topics returns every tier topic of a policy with maxAttempts attempts.
func (n TierNaming) Topics(routeName string, maxAttempts int) []string {
	out := make([]string, 0, maxAttempts)
	for i := 1; i <= maxAttempts; i++ {
		out = append(out, n.Topic(routeName, i))
	}
	return out
}
