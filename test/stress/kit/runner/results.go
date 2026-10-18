package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/scenario"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// SetupResult describes provisioning of a case.
type SetupResult struct {
	Routes  int             `json:"routes"`
	Seconds float64         `json:"seconds"`
	Deploy  latency.Summary `json:"deployToRunning"`
}

// Delivery is the completeness verdict of a case: every acknowledged event must arrive (output or DLQ).
type Delivery struct {
	Acked      uint64 `json:"expected"`
	Unique     uint64 `json:"unique"`
	Lost       int64  `json:"lost"`
	Duplicates uint64 `json:"duplicates"`
	Gaps       uint64 `json:"gaps"`
	ZeroLoss   bool   `json:"zeroLoss"`
	Measured   bool   `json:"measured"`
}

// DeliveryCheck compares producer acks with what the measurer saw.
type DeliveryCheck struct{}

// Verdict computes the delivery result; copies is how many outputs each event must reach (fan-out tracked per topic).
func (DeliveryCheck) Verdict(sums []stream.ProduceSummary, m *stream.MeasureSummary, copies int) Delivery {
	var d Delivery
	for _, s := range sums {
		d.Acked += s.Acked * uint64(max(copies, 1))
	}
	if m == nil {
		return d
	}
	d.Measured = true
	d.Unique, d.Duplicates, d.Gaps = m.Delivery.Unique, m.Delivery.Duplicates, m.Delivery.Gaps
	d.Lost = int64(d.Acked) - int64(d.Unique)
	d.ZeroLoss = d.Lost <= 0 && d.Gaps == 0
	return d
}

// CaseResult is everything recorded for one case.
type CaseResult struct {
	Name       string                  `json:"name"`
	Spec       scenario.Case           `json:"spec"`
	StartedAt  time.Time               `json:"startedAt"`
	FinishedAt time.Time               `json:"finishedAt"`
	Setup      SetupResult             `json:"setup"`
	Knee       knee.Result             `json:"knee"`
	Delivery   Delivery                `json:"delivery"`
	Produce    []stream.ProduceSummary `json:"produce"`
	Measure    *stream.MeasureSummary  `json:"measure,omitempty"`
	Actions    []ActionResult          `json:"actions,omitempty"`
	Timeline   []Point                 `json:"timeline"`
	Cleanup    *CleanupReport          `json:"cleanup,omitempty"`
	Error      string                  `json:"error,omitempty"`
}

// HostInfo describes the machine the runner ran on.
type HostInfo struct {
	Hostname string `json:"hostname"`
	CPUs     int    `json:"cpus"`
}

// ScenarioResult is test/stress/results/<scenario>.json.
type ScenarioResult struct {
	Scenario    string         `json:"scenario"`
	Description string         `json:"description"`
	Target      string         `json:"target"`
	RunBase     string         `json:"runBase"`
	Host        HostInfo       `json:"host"`
	Criteria    knee.Criteria  `json:"criteria"`
	StartedAt   time.Time      `json:"startedAt"`
	FinishedAt  time.Time      `json:"finishedAt"`
	Cases       []CaseResult   `json:"cases"`
	Cleanup     *CleanupReport `json:"cleanup,omitempty"`
}

// ResultWriter writes the JSON result and the short Markdown notes.
type ResultWriter struct {
	Dir string
}

// NewHostInfo describes this host.
func NewHostInfo() HostInfo {
	h, _ := os.Hostname()
	return HostInfo{Hostname: h, CPUs: runtime.NumCPU()}
}

// Write writes <dir>/<scenario>.json and .md.
func (w ResultWriter) Write(r ScenarioResult) (string, error) {
	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(w.Dir, r.Scenario+".json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, os.WriteFile(filepath.Join(w.Dir, r.Scenario+".md"), []byte(Notes{}.Render(r)), 0o644)
}

// Notes renders the Markdown summary of a result.
type Notes struct{}

// Render returns the notes text.
func (n Notes) Render(r ScenarioResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s (%s)\n\n%s\n\nRun `%s` on %s (%d CPUs), %s → %s.\n\n", r.Scenario, r.Target, r.Description, r.RunBase, r.Host.Hostname, r.Host.CPUs, r.StartedAt.Format(time.RFC3339), r.FinishedAt.Format(time.RFC3339))
	b.WriteString("| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Cases {
		p99 := 0.0
		if c.Measure != nil {
			p99 = c.Measure.Latency.P99Ms
		}
		fmt.Fprintf(&b, "| %s | %d | %.0f | %.0f | %s | %.1f | %d | %d | %.0f | %.2f | %s |\n", c.Name, c.Setup.Routes, c.Knee.MaxSustainable, c.Knee.KneeRate, c.Knee.KneeReason, p99, c.Delivery.Lost, c.Delivery.Duplicates, c.Setup.Deploy.P95Ms, n.peakCPU(c), c.Error)
	}
	for _, c := range r.Cases {
		fmt.Fprintf(&b, "\n## %s\n\n| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |\n|---|---|---|---|---|---|---|\n", c.Name)
		for _, s := range c.Knee.Steps {
			fmt.Fprintf(&b, "| %d | %.0f | %.0f | %.1f | %.0f | %.1f | %s |\n", s.Index, s.TargetRate, s.AchievedRate, s.LagSlope, s.LagEnd, s.Latency.P99Ms, s.Verdict)
		}
	}
	if r.Cleanup != nil && len(r.Cleanup.Errors) > 0 {
		fmt.Fprintf(&b, "\nCleanup errors: %s\n", strings.Join(r.Cleanup.Errors, "; "))
	}
	return b.String()
}

func (Notes) peakCPU(c CaseResult) float64 {
	var peak float64
	for _, p := range c.Timeline {
		peak = max(peak, p.WorkerCPUCores)
	}
	return peak
}
