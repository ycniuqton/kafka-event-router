package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryPin is one library reference of a transform step and where it appears.
type LibraryPin struct {
	Kind    LibraryKind
	Name    string
	Version *int
	Step    config.Pointer
}

// LibraryPins collects the library steps (transforms) and named lookup steps (lookup tables) of a
// config, nested if/else steps included.
type LibraryPins struct{}

// Of returns every pin in document order.
func (p LibraryPins) Of(cfg *config.RouteConfig) []LibraryPin {
	out := []LibraryPin{}
	for i, b := range cfg.Branches {
		if b.Transform != nil {
			out = p.collect(b.Transform.Steps, config.BranchPointer(i).Token(config.TokenTransform).Token(config.TokenSteps), out)
		}
	}
	return out
}

// collect appends the pins of steps found under base.
func (p LibraryPins) collect(steps config.Steps, base config.Pointer, out []LibraryPin) []LibraryPin {
	for j, st := range steps {
		at := base.Index(j)
		if kind, ok := p.kindOf(st); ok {
			out = append(out, LibraryPin{Kind: kind, Name: *st.Ref, Version: st.Version, Step: at})
		}
		if st.ThenSteps != nil {
			out = p.collect(*st.ThenSteps, at.Token(config.TokenThenSteps), out)
		}
		if st.ElseSteps != nil {
			out = p.collect(*st.ElseSteps, at.Token(config.TokenElseSteps), out)
		}
	}
	return out
}

// kindOf reports which library a step references: library steps a transform, row-less lookups a table.
func (LibraryPins) kindOf(st config.Step) (LibraryKind, bool) {
	if st.Ref == nil || *st.Ref == "" {
		return "", false
	}
	switch {
	case st.Kind == config.StepKindLibrary:
		return LibraryTransform, true
	case st.Kind == config.StepKindLookup && len(st.Rows) == 0:
		return LibraryLookupTable, true
	}
	return "", false
}

// libraryKindLabel is how issue messages name each kind.
var libraryKindLabel = map[LibraryKind]string{LibraryTransform: "transform", LibraryLookupTable: "lookup table"}

// LibraryRefValidator checks pinned library versions against the team's library: an unknown name or
// version blocks deploy, an older pin than the latest version is a LibraryUpdateAvailable warning.
// It owns library_ref_missing, superseding the compile step's resolver-less findings.
type LibraryRefValidator struct {
	Library LibraryLookup
}

func (LibraryRefValidator) Name() ValidatorName     { return ValidatorLibraryRef }
func (LibraryRefValidator) Modes() []ValidationMode { return AllModes }
func (LibraryRefValidator) OwnsIssues() []config.IssueCode {
	return []config.IssueCode{config.IssueLibraryRefMissing}
}

// Validate resolves each distinct ref once.
func (v LibraryRefValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	versions := &libraryVersions{lookup: v.Library, seen: map[LibraryRef]int{}}
	issues := []config.Issue{}
	for _, pin := range (LibraryPins{}).Of(in.Config) {
		latest, err := versions.latest(ctx, LibraryRef{TeamID: in.TeamID, Kind: pin.Kind, Name: pin.Name})
		if err != nil {
			return nil, err
		}
		issues = append(issues, v.check(pin, latest)...)
	}
	return issues, nil
}

// libraryVersions memoises latest versions for one validation run (0 = unknown name).
type libraryVersions struct {
	lookup LibraryLookup
	seen   map[LibraryRef]int
}

func (c *libraryVersions) latest(ctx context.Context, ref LibraryRef) (int, error) {
	if n, ok := c.seen[ref]; ok {
		return n, nil
	}
	n, ok, err := c.lookup.Latest(ctx, ref)
	if err != nil {
		return 0, err
	}
	if !ok {
		n = 0
	}
	c.seen[ref] = n
	return n, nil
}

// check compares one pin with the latest version (0 = unknown name).
func (LibraryRefValidator) check(pin LibraryPin, latest int) []config.Issue {
	label := libraryKindLabel[pin.Kind]
	at := pin.Step.Token(config.TokenVersion)
	switch {
	case latest == 0:
		return []config.Issue{config.NewError(config.IssueLibraryRefMissing, pin.Step.Token(config.TokenRef),
			"Library %s %q does not exist in the library.", label, pin.Name)}
	case pin.Version == nil:
		return []config.Issue{config.NewError(config.IssueLibraryRefMissing, at, "Pin a version of library %s %q (latest is v%d).", label, pin.Name, latest)}
	case *pin.Version < 1 || *pin.Version > latest:
		return []config.Issue{config.NewError(config.IssueLibraryRefMissing, at,
			"Library %s %q has no version v%d (latest is v%d).", label, pin.Name, *pin.Version, latest)}
	case *pin.Version < latest:
		return []config.Issue{config.NewWarning(config.IssueLibraryUpdateAvailable, at,
			"Library %s %q has a newer version v%d (this route pins v%d).", label, pin.Name, latest, *pin.Version)}
	}
	return nil
}
