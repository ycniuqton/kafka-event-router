package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// IssueReadCommitted warns that exactly-once output is only exactly-once for read_committed consumers.
const IssueReadCommitted config.IssueCode = "exactly_once_read_committed"

// DeliveryModePointer addresses delivery.mode.
var DeliveryModePointer = config.PointerRoot.Token(config.TokenDelivery).Token(config.TokenMode)

// ExactlyOnceValidator completes pipeline-engine's same-connection rule (destinations, PE-09) with the
// dead-letter connection, which route-runtime writes inside the same transaction, and surfaces the
// read_committed requirement for downstream consumers.
type ExactlyOnceValidator struct{}

func (ExactlyOnceValidator) Name() ValidatorName     { return ValidatorExactlyOnce }
func (ExactlyOnceValidator) Modes() []ValidationMode { return AllModes }

// Validate returns nothing for at_least_once routes.
func (v ExactlyOnceValidator) Validate(_ context.Context, in ValidationInput) ([]config.Issue, error) {
	cfg := in.Config
	if cfg == nil || cfg.Delivery == nil || cfg.Delivery.Mode != config.DeliveryModeExactlyOnce {
		return nil, nil
	}
	issues := []config.Issue{config.NewWarning(IssueReadCommitted, DeliveryModePointer,
		"Exactly-once: consumers of the destination topics must read with isolation.level=read_committed, otherwise they also see aborted writes.")}
	if dlq := v.dlqConnection(cfg); dlq != "" && dlq != cfg.Source.ConnectionID {
		issues = append(issues, config.NewError(config.IssueExactlyOnceCrossCluster, DLQConnectionPointer,
			"exactly_once writes dead letters in the same transaction: the dead-letter connection must be the source connection %q", cfg.Source.ConnectionID))
	}
	return issues, nil
}

// dlqConnection is the configured dead-letter connection ("" = the source connection).
func (ExactlyOnceValidator) dlqConnection(cfg *config.RouteConfig) string {
	if cfg.ErrorPolicy == nil || cfg.ErrorPolicy.DLQConnectionID == nil {
		return ""
	}
	return *cfg.ErrorPolicy.DLQConnectionID
}
