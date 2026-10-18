package operators

import "github.com/eventrouter/router/pkg/config"

// Gte builds the `gte` comparison operator.
func Gte() ordering {
	return ordering{op: config.OpNameGte, accepts: func(cmp int) bool { return cmp >= 0 }}
}
