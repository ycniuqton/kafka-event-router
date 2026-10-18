package operators

import "github.com/eventrouter/router/pkg/config"

// Lt builds the `lt` comparison operator.
func Lt() ordering {
	return ordering{op: config.OpNameLt, accepts: func(cmp int) bool { return cmp < 0 }}
}
