package assisthttp_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	assisthttp "github.com/eventrouter/router/internal/assist/http"
)

// staticToggle is a fixed workspace AI-assist setting.
type staticToggle bool

func (t staticToggle) AIAssistEnabled(context.Context) (bool, error) { return bool(t), nil }

func TestGateFollowsWorkspaceToggle(t *testing.T) {
	r := httptest.NewRequest("POST", string(assisthttp.PathSteps), nil)
	if err := (assisthttp.RateGate{Toggle: staticToggle(false)}).Check(httptest.NewRecorder(), r); !errors.Is(err, assist.ErrAssistOff) {
		t.Fatalf("off: err = %v, want ErrAssistOff", err)
	}
	if err := (assisthttp.RateGate{Toggle: staticToggle(true)}).Check(httptest.NewRecorder(), r); err != nil {
		t.Fatalf("on: err = %v", err)
	}
	if err := (assisthttp.RateGate{}).Check(httptest.NewRecorder(), r); err != nil {
		t.Fatalf("no toggle: err = %v", err)
	}
}
