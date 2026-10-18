package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// DestinationTopicValidator checks destination topics exist unless the branch creates them.
type DestinationTopicValidator struct {
	Inspector TopicInspector
}

func (DestinationTopicValidator) Name() ValidatorName { return ValidatorDestinationTopic }
func (DestinationTopicValidator) Modes() []ValidationMode {
	return []ValidationMode{ModeDeploy, ModeRollback}
}

// Validate: missing ∧ no createTopic → error; cluster unreachable → warning.
func (v DestinationTopicValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	issues := []config.Issue{}
	for i, b := range in.Config.Branches {
		if b.Destination.CreateTopic != nil || b.Destination.Topic == "" || b.Destination.ConnectionID == "" {
			continue
		}
		path := config.BranchPointer(i).Token(config.TokenDestination).Key(string(MemberTopic))
		exists, err := v.Inspector.TopicExists(ctx, b.Destination.ConnectionID, b.Destination.Topic)
		if err != nil {
			issues = append(issues, config.NewWarning(IssueCheckSkipped, path, "Could not reach the destination cluster to check topic %q.", b.Destination.Topic))
			continue
		}
		if !exists {
			issues = append(issues, config.NewError(config.IssueTopicMissing, path, "Topic %q does not exist. Create it or enable \"Create topic if missing\".", b.Destination.Topic))
		}
	}
	return issues, nil
}
