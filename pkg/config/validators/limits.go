package validators

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// LimitsValidator checks the cross-field autotune limits (in-flight floor vs cap, JS pool min vs max).
type LimitsValidator struct{}

func (LimitsValidator) Name() config.ValidatorName { return config.ValidatorLimits }

func (LimitsValidator) Validate(_ context.Context, vc *config.ValidationContext) []config.Issue {
	cfg, err := vc.Doc.Config()
	if err != nil {
		return nil
	}
	return config.LimitRules{}.Check(cfg.Limits)
}
