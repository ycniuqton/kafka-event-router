package operators

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// After builds the `after` operator (strictly later).
func After() temporal {
	return temporal{op: config.OpNameAfter, test: func(l, o time.Time) bool { return l.After(o) }}
}
