package operators

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// Before builds the `before` operator (strictly earlier).
func Before() temporal {
	return temporal{op: config.OpNameBefore, test: func(l, o time.Time) bool { return l.Before(o) }}
}
