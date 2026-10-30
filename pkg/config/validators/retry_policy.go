package validators

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// RetryPolicyValidator checks the cross-field bound of errorPolicy.retry: maxDelayMs ≥ baseDelayMs (defaults applied).
type RetryPolicyValidator struct{}

func (RetryPolicyValidator) Name() config.ValidatorName { return config.ValidatorRetryPolicy }

func (RetryPolicyValidator) Validate(_ context.Context, vc *config.ValidationContext) []config.Issue {
	cfg, err := vc.Doc.Config()
	if err != nil || cfg.ErrorPolicy == nil || cfg.ErrorPolicy.Retry == nil {
		return nil
	}
	p := config.RetryPolicyOf(cfg.ErrorPolicy)
	if p.MaxDelay >= p.BaseDelay {
		return nil
	}
	ptr := config.PointerRoot.Token(config.TokenErrorPolicy).Token(config.TokenRetry).Token(config.TokenMaxDelayMs)
	return []config.Issue{config.NewError(config.IssueLimitOutOfRange, ptr,
		"maxDelayMs (%d) must be at least baseDelayMs (%d)", p.MaxDelay.Milliseconds(), p.BaseDelay.Milliseconds())}
}
