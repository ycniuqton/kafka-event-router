package operators

import "github.com/eventrouter/router/pkg/config"

// Lte builds the `lte` comparison operator.
func Lte() ordering {
	return ordering{op: config.OpNameLte, accepts: func(cmp int) bool { return cmp <= 0 }}
}
