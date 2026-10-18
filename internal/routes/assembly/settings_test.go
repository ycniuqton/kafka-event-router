package assembly

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/pkg/engine/script"
)

func TestSettingsDefaultsAndOverrides(t *testing.T) {
	s, err := SettingsLoader{Lookup: func(string) (string, bool) { return "", false }}.Load()
	if err != nil || s.ApprovalPolicy != routes.ApprovalAuto || s.MetricsReader != routes.MetricsReaderNull ||
		s.ScriptCheck != ScriptCheckSandbox || s.ValidationTimeout != 5*time.Second || s.ValidationParallel != 4 {
		t.Fatalf("%+v %v", s, err)
	}
	env := map[string]string{string(EnvScriptCheck): "skip", string(EnvValidationTimeout): "2s"}
	s, _ = SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}.Load()
	if s.ScriptCheck != ScriptCheckSkip || s.ValidationTimeout != 2*time.Second {
		t.Fatalf("%+v", s)
	}
	env[string(EnvValidationParallel)] = "x"
	if _, err := (SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}).Load(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestScriptCheckerSelection(t *testing.T) {
	if _, err := (ScriptCheckerSelector{}).Select(context.Background(), "bogus"); err == nil {
		t.Fatal("unknown kind must fail")
	}
	c, err := ScriptCheckerSelector{}.Select(context.Background(), ScriptCheckSkip)
	if err != nil || c.Kind() != ScriptCheckSkip {
		t.Fatal(err)
	}
	mod, err := c.Runtime().Compile(context.Background(), script.Source{Code: "x("}, script.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.Call(context.Background(), script.Invocation{}); err != ErrNotExecutable {
		t.Fatal(err)
	}
}
