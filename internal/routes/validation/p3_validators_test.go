package validation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine/compile"
)

// docChecker is pipeline-engine's doc-only profile (schema, delivery mode, compile).
func docChecker(t *testing.T) DocumentChecker {
	t.Helper()
	m, err := validators.NewDefaultManager(validators.Deps{Compiler: compile.NewDefaultCompiler()})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// codesAt maps issue paths to codes.
func codesAt(issues []config.Issue) map[config.Pointer]config.IssueCode {
	out := map[config.Pointer]config.IssueCode{}
	for _, is := range issues {
		out[is.Path] = is.Code
	}
	return out
}

const eoDoc = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"json"}},
"branches":[{"id":"main","destination":{"connectionId":"%s","topic":"out"}}],
"errorPolicy":{"onError":"dlq"%s},"delivery":{"mode":"%s"}}`

func eo(dest, dlq, mode string) string {
	extra := ""
	if dlq != "" {
		extra = `,"dlqConnectionId":"` + dlq + `"`
	}
	return fmt.Sprintf(eoDoc, dest, extra, mode)
}

func TestExactlyOnceValidator(t *testing.T) {
	v := ExactlyOnceValidator{}
	got, _ := v.Validate(context.Background(), input(t, ModeDeploy, eo("c1", "", "at_least_once")))
	if len(got) != 0 {
		t.Fatalf("at_least_once: %+v", got)
	}
	got, _ = v.Validate(context.Background(), input(t, ModeDeploy, eo("c1", "c1", "exactly_once")))
	if len(got) != 1 || got[0].Code != IssueReadCommitted || got[0].Severity != config.SeverityWarning || got[0].Path != "/delivery/mode" {
		t.Fatalf("same connection: %+v", got)
	}
	got, _ = v.Validate(context.Background(), input(t, ModeDeploy, eo("c1", "c2", "exactly_once")))
	if c := codesAt(got); c[DLQConnectionPointer] != config.IssueExactlyOnceCrossCluster || !config.Issues(got).HasErrors() {
		t.Fatalf("cross-connection DLQ: %+v", got)
	}
}

// TestExactlyOnceCrossConnectionRejectedOnDeploy runs the full deploy profile: pipeline-engine's
// destination rule (PE-09) and the DLQ rule both block, the read_committed warning stays a warning.
func TestExactlyOnceCrossConnectionRejectedOnDeploy(t *testing.T) {
	docs := docChecker(t)
	m := NewValidationManager(NewValidatorSet(DocumentValidator{Checker: docs}, ExactlyOnceValidator{}), ManagerOptions{})
	res := m.Run(context.Background(), Request{Mode: ModeDeploy, TeamID: "t1", Raw: []byte(eo("c2", "c3", "exactly_once"))})
	c := codesAt(res.Issues)
	if res.Valid || c["/branches/0/destination/connectionId"] != config.IssueExactlyOnceCrossCluster || c[DLQConnectionPointer] != config.IssueExactlyOnceCrossCluster {
		t.Fatalf("cross-connection exactly_once must be rejected: %+v", res.Issues)
	}
	res = m.Run(context.Background(), Request{Mode: ModeDeploy, TeamID: "t1", Raw: []byte(eo("c1", "", "exactly_once"))})
	if !res.Valid || codesAt(res.Issues)["/delivery/mode"] != IssueReadCommitted {
		t.Fatalf("same-connection exactly_once is deployable with a warning: %+v", res.Issues)
	}
}

// fakeLibrary holds latest versions by kind/name; calls counts lookups.
type fakeLibrary struct {
	latest map[LibraryRef]int
	calls  int
	err    error
}

func (f *fakeLibrary) Latest(_ context.Context, ref LibraryRef) (int, bool, error) {
	f.calls++
	n, ok := f.latest[ref]
	return n, ok, f.err
}

const libDoc = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"json"}},
"branches":[{"id":"main","destination":{"connectionId":"c1","topic":"out"},"transform":{"steps":[
  {"kind":"library","ref":"clean","version":2},
  {"kind":"library","ref":"clean","version":1},
  {"kind":"library","ref":"gone","version":1},
  {"kind":"lookup","field":"value.c","to":"value.n","ref":"countries","version":9},
  {"kind":"lookup","field":"value.c","to":"value.n","rows":[{"key":"a","value":"b"}]},
  {"kind":"if_else","condition":{"type":"cond","field":"value.a","op":"exists"},
   "thenSteps":[{"kind":"library","ref":"clean","version":3}]}]}}]}`

func TestLibraryRefValidator(t *testing.T) {
	lib := &fakeLibrary{latest: map[LibraryRef]int{
		{TeamID: "team-a", Kind: LibraryTransform, Name: "clean"}:       2,
		{TeamID: "team-a", Kind: LibraryLookupTable, Name: "countries"}: 4,
	}}
	got, err := LibraryRefValidator{Library: lib}.Validate(context.Background(), input(t, ModeDeploy, libDoc))
	if err != nil {
		t.Fatal(err)
	}
	want := map[config.Pointer]config.IssueCode{
		"/branches/0/transform/steps/1/version":             config.IssueLibraryUpdateAvailable,
		"/branches/0/transform/steps/2/ref":                 config.IssueLibraryRefMissing,
		"/branches/0/transform/steps/3/version":             config.IssueLibraryRefMissing,
		"/branches/0/transform/steps/5/thenSteps/0/version": config.IssueLibraryRefMissing,
	}
	c := codesAt(got)
	if len(got) != len(want) {
		t.Fatalf("issues %+v", got)
	}
	for p, code := range want {
		if c[p] != code {
			t.Errorf("%s: got %s want %s", p, c[p], code)
		}
	}
	if lib.calls != 3 {
		t.Fatalf("each distinct ref is looked up once, got %d calls", lib.calls)
	}
	lib.err = errors.New("db down")
	if _, err := (LibraryRefValidator{Library: lib}).Validate(context.Background(), input(t, ModeDeploy, libDoc)); err == nil {
		t.Fatal("lookup errors must surface as a skipped check")
	}
}

// ownerStub reports a fixed issue and owns its code; skip makes it time out.
type ownerStub struct {
	issue config.Issue
	skip  bool
}

func (ownerStub) Name() ValidatorName     { return ValidatorLibraryRef }
func (ownerStub) Modes() []ValidationMode { return AllModes }
func (ownerStub) OwnsIssues() []config.IssueCode {
	return []config.IssueCode{config.IssueLibraryRefMissing}
}
func (o ownerStub) Validate(ctx context.Context, _ ValidationInput) ([]config.Issue, error) {
	if o.skip {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []config.Issue{o.issue}, nil
}

// fixed returns its issues.
type fixed []config.Issue

func (fixed) Name() ValidatorName     { return ValidatorDocument }
func (fixed) Modes() []ValidationMode { return AllModes }
func (f fixed) Validate(context.Context, ValidationInput) ([]config.Issue, error) {
	return f, nil
}

func TestIssueOwnerSupersedesOtherValidators(t *testing.T) {
	compile := config.NewError(config.IssueLibraryRefMissing, "/branches/0/transform/steps/0/ref", "no library is available")
	other := config.NewWarning(IssueNameMismatch, "/name", "x")
	mine := config.NewWarning(config.IssueLibraryUpdateAvailable, "/branches/0/transform/steps/0/version", "newer")
	m := NewValidationManager(NewValidatorSet(fixed{compile, other}, ownerStub{issue: mine}), ManagerOptions{})
	res := m.Run(context.Background(), Request{Mode: ModeDraft, Raw: []byte(`{}`)})
	if !res.Valid || len(res.Issues) != 2 {
		t.Fatalf("compile's library issue must be superseded: %+v", res.Issues)
	}
	m = NewValidationManager(NewValidatorSet(fixed{compile}, ownerStub{skip: true}), ManagerOptions{Timeout: 10 * time.Millisecond})
	res = m.Run(context.Background(), Request{Mode: ModeDraft, Raw: []byte(`{}`)})
	if res.Valid || len(res.Issues) != 2 {
		t.Fatalf("a skipped owner supersedes nothing: %+v", res.Issues)
	}
}
