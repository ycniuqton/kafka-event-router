package assist_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/internal/assist/mock"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/pkg/config"
)

func str(s string) *string { return &s }

func newProposer(t *testing.T, p assist.LLMProvider) *assist.StepProposer {
	t.Helper()
	checker, err := assist.NewStepChecker()
	if err != nil {
		t.Fatal(err)
	}
	return assist.NewStepProposer(assist.NewProviderManager(p, assist.DefaultLimits()), checker, describe.DefaultDescriberManager())
}

// fixedProvider returns canned steps (to exercise policy and validation).
type fixedProvider struct {
	steps string
	err   error
	delay time.Duration
}

func (fixedProvider) Kind() assist.ProviderKind { return "fixed" }
func (f fixedProvider) Complete(ctx context.Context, _ assist.Prompt) (assist.Completion, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return assist.Completion{}, ctx.Err()
		}
	}
	return assist.Completion{Steps: json.RawMessage(f.steps)}, f.err
}

func TestProposerWithMockProvider(t *testing.T) {
	p := newProposer(t, mock.NewMockProvider())
	got, err := p.Propose(context.Background(), assist.ProposalRequest{
		Instruction: "mask email and rename cust_id to customerId", Fields: []string{"value.email", "value.cust_id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 || got.Provider != assist.ProviderMock || got.ID == "" {
		t.Fatalf("unexpected proposal %+v", got)
	}
	want := []string{"Mask email (email style)", "Rename cust_id → customerId"}
	for i, line := range want {
		if got.Added[i] != line {
			t.Errorf("diff line %d = %q, want %q", i, got.Added[i], line)
		}
	}
	if len(got.Issues) != 0 {
		t.Errorf("unexpected issues %+v", got.Issues)
	}
}

func TestProposerRejectsBadInstructions(t *testing.T) {
	p := newProposer(t, mock.NewMockProvider())
	cases := map[string]struct {
		req  assist.ProposalRequest
		want error
	}{
		"empty":    {assist.ProposalRequest{Instruction: "  "}, assist.ErrInstructionEmpty},
		"too long": {assist.ProposalRequest{Instruction: strings.Repeat("x", 2001)}, assist.ErrInstructionTooLong},
		"fields":   {assist.ProposalRequest{Instruction: "mask a", Fields: make([]string, 501)}, assist.ErrTooManyFields},
	}
	for name, tc := range cases {
		if _, err := p.Propose(context.Background(), tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestProposerValidatesProviderSteps(t *testing.T) {
	p := newProposer(t, fixedProvider{steps: `[{"kind":"mask","field":"value.email"},{"kind":"rename","from":"value.a[","to":"value.b"},{"kind":"teleport"}]`})
	got, err := p.Propose(context.Background(), assist.ProposalRequest{Instruction: "anything"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("unknown kind not stripped: %+v", got.Steps)
	}
	paths := map[config.Pointer]config.Severity{}
	for _, i := range got.Issues {
		paths[i.Path] = i.Severity
	}
	if paths["/steps/0"] != config.SeverityError {
		t.Errorf("missing style not reported at /steps/0: %+v", got.Issues)
	}
	if paths["/steps/1/from"] != config.SeverityError {
		t.Errorf("bad path not reported at /steps/1/from: %+v", got.Issues)
	}
	if paths["/steps"] != config.SeverityWarning {
		t.Errorf("stripped kind not warned: %+v", got.Issues)
	}
}

func TestProviderFailuresAreUnavailable(t *testing.T) {
	for name, prov := range map[string]fixedProvider{
		"error":   {err: errors.New("boom")},
		"garbage": {steps: `{"not":"an array"}`},
	} {
		_, err := newProposer(t, prov).Propose(context.Background(), assist.ProposalRequest{Instruction: "x"})
		if !errors.Is(err, assist.ErrUnavailable) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestProviderManagerTimeout(t *testing.T) {
	limits := assist.DefaultLimits()
	limits.Timeout = 10 * time.Millisecond
	rec := &recordingMetrics{}
	m := assist.NewProviderManager(fixedProvider{steps: `[]`, delay: time.Second}, limits).WithMetrics(rec)
	if _, err := m.Complete(context.Background(), assist.Prompt{}); !errors.Is(err, assist.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	ok := assist.NewProviderManager(fixedProvider{steps: `[]`}, assist.DefaultLimits()).WithMetrics(rec)
	if _, err := ok.Complete(context.Background(), assist.Prompt{}); err != nil {
		t.Fatal(err)
	}
	if len(rec.results) != 2 || rec.results[0] != assist.ResultError || rec.results[1] != assist.ResultOK {
		t.Fatalf("observed %v", rec.results)
	}
	assist.NewPromMetrics(nil).Observe(assist.TaskExplain, assist.ProviderMock, assist.ResultOK, time.Millisecond)
}

func TestProposalPolicy(t *testing.T) {
	js := config.Step{Kind: config.StepKindJS, Code: str("function transform(e){return e}")}
	rename := config.Step{Kind: config.StepKindRename, From: str("value.a"), To: str("value.b")}
	policy := assist.NewProposalPolicy(assist.RecipeKinds, 2)

	kept, issues := policy.Apply([]config.Step{js, rename})
	if len(kept) != 1 || kept[0].Kind != config.StepKindRename || len(issues) != 1 {
		t.Errorf("JS not dropped next to recipes: %+v %+v", kept, issues)
	}
	kept, _ = policy.Apply([]config.Step{js})
	if len(kept) != 1 {
		t.Errorf("JS-only proposal must stay: %+v", kept)
	}
	kept, issues = policy.Apply([]config.Step{rename, rename, rename, {Kind: config.StepKindLibrary}})
	if len(kept) != 2 || len(issues) != 2 {
		t.Errorf("cap/strip failed: %d kept, issues %+v", len(kept), issues)
	}
}

func TestRegistryBuild(t *testing.T) {
	reg := assist.NewProviderRegistry()
	mock.Register(reg)
	s := assist.Settings{Provider: assist.ProviderMock, Limits: assist.DefaultLimits()}
	if m, err := reg.Build(s); err != nil || m.Kind() != assist.ProviderMock {
		t.Fatalf("mock build: %v", err)
	}
	s.Provider = "openai"
	if _, err := reg.Build(s); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{}
	l := assist.SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}
	s, err := l.Load()
	if err != nil || s.Provider != assist.ProviderMock || s.RatePerMinute != 30 || s.Timeout != 20*time.Second {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	env[string(assist.EnvAssistTimeout)] = "soon"
	if _, err := l.Load(); err == nil {
		t.Fatal("bad duration accepted")
	}
}

func TestSettingsInferMatchers(t *testing.T) {
	env := map[string]string{string(assist.EnvAssistInferMatchers): " name, value ,"}
	s, err := assist.SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}.Load()
	if err != nil || len(s.InferMatchers) != 2 || s.InferMatchers[0] != "name" {
		t.Fatalf("matchers %v %v", s.InferMatchers, err)
	}
}

func TestWindowLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := assist.NewWindowLimiter(2, assist.MinuteWindow, func() time.Time { return now })
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow(context.Background(), "u"); !ok {
			t.Fatalf("call %d refused", i)
		}
	}
	ok, wait := l.Allow(context.Background(), "u")
	if ok || wait != time.Minute {
		t.Fatalf("third call: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow(context.Background(), "other"); !ok {
		t.Fatal("limit is per key")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow(context.Background(), "u"); !ok {
		t.Fatal("window did not reset")
	}
}

type fakePlatformLimiter struct {
	decision ratelimit.Decision
	err      error
	key      string
}

func (f *fakePlatformLimiter) Allow(_ context.Context, key string, _ ratelimit.Rule) (ratelimit.Decision, error) {
	f.key = key
	return f.decision, f.err
}

func TestSharedLimiter(t *testing.T) {
	f := &fakePlatformLimiter{decision: ratelimit.Decision{Allowed: false, RetryAfter: 3 * time.Second}}
	ok, wait := assist.NewSharedLimiter(f, 30).Allow(context.Background(), "u1")
	if ok || wait != 3*time.Second || f.key != string(assist.AssistBucket)+"u1" {
		t.Fatalf("refusal: ok=%v wait=%v key=%q", ok, wait, f.key)
	}
	f.err = errors.New("redis down")
	if ok, _ := assist.NewSharedLimiter(f, 30).Allow(context.Background(), "u1"); !ok {
		t.Fatal("limiter must fail open")
	}
}

type recordingMetrics struct{ results []assist.RequestResult }

func (r *recordingMetrics) Observe(_ assist.AssistTask, _ assist.ProviderKind, res assist.RequestResult, _ time.Duration) {
	r.results = append(r.results, res)
}
