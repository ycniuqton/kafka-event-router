package validation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

type fakeValidator struct {
	name   ValidatorName
	modes  []ValidationMode
	issues []config.Issue
	err    error
	delay  time.Duration
	calls  *int
}

func (f fakeValidator) Name() ValidatorName     { return f.name }
func (f fakeValidator) Modes() []ValidationMode { return f.modes }
func (f fakeValidator) Validate(ctx context.Context, _ ValidationInput) ([]config.Issue, error) {
	if f.calls != nil {
		*f.calls++
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.issues, f.err
}

const validDoc = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"json"}},"branches":[{"id":"main","destination":{"connectionId":"c2","topic":"out"}}]}`

func TestManagerFiltersByMode(t *testing.T) {
	draftCalls, deployCalls := 0, 0
	set := NewValidatorSet(
		fakeValidator{name: "draft-only", modes: []ValidationMode{ModeDraft}, calls: &draftCalls},
		fakeValidator{name: "deploy-only", modes: []ValidationMode{ModeDeploy}, calls: &deployCalls},
	)
	m := NewValidationManager(set, ManagerOptions{})
	m.Run(context.Background(), Request{Mode: ModeDraft, Raw: []byte(validDoc)})
	if draftCalls != 1 || deployCalls != 0 {
		t.Fatalf("draft=%d deploy=%d", draftCalls, deployCalls)
	}
}

func TestManagerTimeoutAndErrorBecomeWarnings(t *testing.T) {
	set := NewValidatorSet(
		fakeValidator{name: "slow", modes: AllModes, delay: time.Second},
		fakeValidator{name: "broken", modes: AllModes, err: errors.New("kafka down")},
	)
	m := NewValidationManager(set, ManagerOptions{Timeout: 20 * time.Millisecond})
	res := m.Run(context.Background(), Request{Mode: ModeDeploy, Raw: []byte(validDoc)})
	if !res.Valid || len(res.Issues) != 2 {
		t.Fatalf("%+v", res)
	}
	for _, is := range res.Issues {
		if is.Code != IssueCheckSkipped || is.Severity != config.SeverityWarning {
			t.Fatalf("unexpected issue %+v", is)
		}
	}
}

func TestManagerSortsAndBlocksOnErrors(t *testing.T) {
	set := NewValidatorSet(
		fakeValidator{name: "a", modes: AllModes, issues: []config.Issue{config.NewWarning(config.IssueFieldUnknown, "/branches/0/filter/field", "w")}},
		fakeValidator{name: "b", modes: AllModes, issues: []config.Issue{config.NewError(config.IssueConnectionNotFound, "/branches/0/destination/connectionId", "e")}},
		fakeValidator{name: "c", modes: AllModes, issues: []config.Issue{config.NewError(config.IssueConnectionNotFound, "/source/connectionId", "e")}},
	)
	res := NewValidationManager(set, ManagerOptions{Parallelism: 1}).Run(context.Background(), Request{Mode: ModeDeploy, Raw: []byte(validDoc)})
	if res.Valid {
		t.Fatal("errors must block")
	}
	want := []config.Pointer{"/branches/0/destination/connectionId", "/branches/0/filter/field", "/source/connectionId"}
	for i, p := range want {
		if res.Issues[i].Path != p {
			t.Fatalf("issue %d at %s, want %s", i, res.Issues[i].Path, p)
		}
	}
	warnOnly := NewValidatorSet(set.validators[0])
	if !NewValidationManager(warnOnly, ManagerOptions{}).Run(context.Background(), Request{Mode: ModeDeploy, Raw: []byte(validDoc)}).Valid {
		t.Fatal("warnings never block")
	}
}

func TestInputBuilderKeepsUnparseableDocumentRaw(t *testing.T) {
	in := InputBuilder{}.Build(Request{Mode: ModeDraft, Raw: []byte(`{"broken"`)})
	if in.Doc != nil || in.Config != nil {
		t.Fatal("expected nil doc")
	}
	is, _ := DocumentValidator{}.Validate(context.Background(), in)
	if len(is) != 1 || is[0].Code != config.IssueSchemaInvalid {
		t.Fatalf("%+v", is)
	}
}
