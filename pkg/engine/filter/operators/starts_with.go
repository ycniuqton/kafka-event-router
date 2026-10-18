package operators

import (
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// StartsWith builds the `starts_with` operator.
func StartsWith() textMatch {
	return textMatch{op: config.OpNameStartsWith, test: strings.HasPrefix}
}
