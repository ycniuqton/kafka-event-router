package operators

import "github.com/eventrouter/router/pkg/config"

// Gt builds the `gt` comparison operator.
func Gt() ordering {
	return ordering{op: config.OpNameGt, accepts: func(cmp int) bool { return cmp > 0 }}
}
