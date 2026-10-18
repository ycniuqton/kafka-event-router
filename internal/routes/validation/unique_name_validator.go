package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// NamePointer addresses the document's name member.
var NamePointer = config.PointerRoot.Token(config.TokenName)

// UniqueNameValidator keeps the document name aligned with the route and unique in the team.
type UniqueNameValidator struct {
	Index RouteNameIndex
}

func (UniqueNameValidator) Name() ValidatorName     { return ValidatorUniqueName }
func (UniqueNameValidator) Modes() []ValidationMode { return AllModes }

// Validate: name used by another team route → error; differs from the route name → warning.
func (v UniqueNameValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil || in.Config.Name == "" {
		return nil, nil
	}
	if in.RouteName != "" && in.Config.Name != in.RouteName {
		return []config.Issue{config.NewWarning(IssueNameMismatch, NamePointer,
			"The document name %q differs from the route name %q; the route name is used.", in.Config.Name, in.RouteName)}, nil
	}
	taken, err := v.Index.NameTakenByOther(ctx, in.TeamID, in.Config.Name, in.RouteID)
	if err != nil || !taken {
		return nil, err
	}
	return []config.Issue{config.NewError(IssueNameTaken, NamePointer, "Another route of this team is already named %q.", in.Config.Name)}, nil
}
