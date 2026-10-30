package main

import (
	"testing"

	"github.com/eventrouter/router/internal/runtime/membudget"
)

// nopPauser ignores budget pauses (the test reads the pressure signal only).
type nopPauser struct{}

func (nopPauser) PauseFetch()  {}
func (nopPauser) ResumeFetch() {}

func TestInFlightTuningUsesTheWorkerMemoryBudgetAsPressure(t *testing.T) {
	budget := membudget.New(100)
	tuning := Assembly{}.inFlightTuning(budget)
	if tuning.Pressure == nil || tuning.Pressure.UnderPressure() {
		t.Fatal("idle budget must be the pressure signal and report none")
	}
	acct := budget.Open("r1", nopPauser{})
	defer acct.Close()
	acct.Charge(100)
	if !tuning.Pressure.UnderPressure() {
		t.Fatal("budget over its high watermark must report pressure to the in-flight tuner")
	}
}
