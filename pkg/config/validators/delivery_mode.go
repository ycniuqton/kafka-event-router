package validators

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// DeliveryModeValidator allows exactly_once only when every destination uses the source connection.
type DeliveryModeValidator struct{}

func (DeliveryModeValidator) Name() config.ValidatorName { return config.ValidatorDeliveryMode }

func (DeliveryModeValidator) Validate(_ context.Context, vc *config.ValidationContext) []config.Issue {
	cfg, err := vc.Doc.Config()
	if err != nil || cfg.Delivery == nil || cfg.Delivery.Mode != config.DeliveryModeExactlyOnce {
		return nil
	}
	issues := []config.Issue{}
	for i, b := range cfg.Branches {
		if b.Destination.ConnectionID != cfg.Source.ConnectionID {
			ptr := config.BranchPointer(i).Token(config.TokenDestination).Token(config.TokenConnection)
			issues = append(issues, config.NewError(config.IssueExactlyOnceCrossCluster, ptr,
				"exactly_once requires the destination to use the source connection %q", cfg.Source.ConnectionID))
		}
	}
	return issues
}
