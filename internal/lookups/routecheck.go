package lookups

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/enrich"
	"github.com/eventrouter/router/pkg/lookup"
)

// ValidatorLookupSource names the route validator of lookup-source references.
const ValidatorLookupSource validation.ValidatorName = "lookup_source"

// SourceRefMessage holds the issue texts of the reference check.
type SourceRefMessage string

const (
	MsgSourceMissing   SourceRefMessage = "Lookup source %q does not exist. Pick a source on the Connections screen."
	MsgSourceForbidden SourceRefMessage = "Lookup source %q belongs to another team. Use a source of this route's team."
	MsgSourceKind      SourceRefMessage = "A %s request cannot call the %s source %q. Pick a %s source or change the request kind."
)

// SourceRef is one lookup step's source reference and where it appears.
type SourceRef struct {
	ID      string
	Request config.LookupRequestKind
	Pointer config.Pointer
}

// SourceRefs collects the source-mode lookup steps of a config (nested if/else steps included).
type SourceRefs struct{}

// Of returns every reference in document order.
func (r SourceRefs) Of(cfg *config.RouteConfig) []SourceRef {
	var out []SourceRef
	for i, b := range cfg.Branches {
		if b.Transform != nil {
			out = r.collect(b.Transform.Steps, config.BranchPointer(i).Token(config.TokenTransform).Token(config.TokenSteps), out)
		}
	}
	return out
}

func (r SourceRefs) collect(steps config.Steps, base config.Pointer, out []SourceRef) []SourceRef {
	for j, st := range steps {
		at := base.Index(j)
		if st.Kind == config.StepKindLookup && st.Lookup != nil {
			out = append(out, SourceRef{ID: st.Lookup.SourceID, Request: st.Lookup.Request.Kind, Pointer: at.Token(enrich.TokenLookup).Token(enrich.TokenSourceID)})
		}
		if st.ThenSteps != nil {
			out = r.collect(*st.ThenSteps, at.Token(config.TokenThenSteps), out)
		}
		if st.ElseSteps != nil {
			out = r.collect(*st.ElseSteps, at.Token(config.TokenElseSteps), out)
		}
	}
	return out
}

// RequestSourceKind is the source kind each request kind needs.
var RequestSourceKind = map[config.LookupRequestKind]lookup.SourceKind{
	config.LookupRequestKindHttp:   lookup.KindHTTP,
	config.LookupRequestKindRedis:  lookup.KindRedis,
	config.LookupRequestKindSql:    lookup.KindPostgres,
	config.LookupRequestKindStatic: lookup.KindStatic,
}

// SourceRefValidator is the routes external-data rule: referenced sources exist, belong to the route's team
// and match the request kind.
type SourceRefValidator struct {
	Repo Repository
}

func (SourceRefValidator) Name() validation.ValidatorName { return ValidatorLookupSource }

func (SourceRefValidator) Modes() []validation.ValidationMode { return validation.AllModes }

// Validate implements validation.ContextValidator; store failures mean "could not check".
func (v SourceRefValidator) Validate(ctx context.Context, in validation.ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	var issues []config.Issue
	for _, ref := range (SourceRefs{}).Of(in.Config) {
		issue, err := v.check(ctx, in.TeamID, ref)
		if err != nil {
			return nil, err
		}
		if issue != nil {
			issues = append(issues, *issue)
		}
	}
	return issues, nil
}

func (v SourceRefValidator) check(ctx context.Context, team string, ref SourceRef) (*config.Issue, error) {
	id, err := uuid.Parse(ref.ID)
	src := Source{}
	if err == nil {
		src, err = v.Repo.Get(ctx, id)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) && id != uuid.Nil {
		return nil, err
	}
	switch {
	case err != nil:
		issue := config.NewError(config.IssueConnectionNotFound, ref.Pointer, string(MsgSourceMissing), ref.ID)
		return &issue, nil
	case team != "" && src.TeamID.String() != team:
		issue := config.NewError(config.IssueConnectionForbidden, ref.Pointer, string(MsgSourceForbidden), src.Name)
		return &issue, nil
	case RequestSourceKind[ref.Request] != src.Kind:
		want := RequestSourceKind[ref.Request]
		issue := config.NewError(config.IssueSchemaInvalid, ref.Pointer, string(MsgSourceKind), ref.Request, src.Kind, src.Name, want)
		return &issue, nil
	}
	return nil, nil
}
