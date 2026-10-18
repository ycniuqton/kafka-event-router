package operators

import (
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// EndsWith builds the `ends_with` operator.
func EndsWith() textMatch {
	return textMatch{op: config.OpNameEndsWith, test: strings.HasSuffix}
}
