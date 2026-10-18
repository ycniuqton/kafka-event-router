package runner

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// ActionLimits bound timed actions.
var ActionLimits = struct {
	Timeout time.Duration
	Output  int
}{Timeout: 5 * time.Minute, Output: 2000}

// ActionResult records one executed action.
type ActionResult struct {
	Name       string  `json:"name"`
	AtSec      float64 `json:"atSec"`
	FiredAtSec float64 `json:"firedAtSec"`
	Seconds    float64 `json:"seconds"`
	Output     string  `json:"output,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// ActionScheduler fires scenario actions (sh -c) at their offsets from the start of the load, asynchronously.
type ActionScheduler struct {
	pending []scenario.ActionSpec
	start   time.Time
	log     func(string, ...any)
	mu      sync.Mutex
	wg      sync.WaitGroup
	results []ActionResult
}

// NewActionScheduler orders actions by offset.
func NewActionScheduler(actions []scenario.ActionSpec, start time.Time, log func(string, ...any)) *ActionScheduler {
	pending := append([]scenario.ActionSpec{}, actions...)
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].At < pending[j].At })
	return &ActionScheduler{pending: pending, start: start, log: log}
}

// Fire starts every action whose offset has passed.
func (a *ActionScheduler) Fire(ctx context.Context, now time.Time) {
	for len(a.pending) > 0 && now.Sub(a.start) >= a.pending[0].At {
		act := a.pending[0]
		a.pending = a.pending[1:]
		a.wg.Add(1)
		go a.run(ctx, act, now)
	}
}

// Wait waits for running actions and returns all results.
func (a *ActionScheduler) Wait() []ActionResult {
	a.wg.Wait()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.results
}

func (a *ActionScheduler) run(ctx context.Context, act scenario.ActionSpec, now time.Time) {
	defer a.wg.Done()
	a.log("action %s at +%s: %s", act.Name, act.At, act.Run)
	ctx, cancel := context.WithTimeout(ctx, ActionLimits.Timeout)
	defer cancel()
	began := time.Now()
	out, err := exec.CommandContext(ctx, "sh", "-c", act.Run).CombinedOutput()
	r := ActionResult{Name: act.Name, AtSec: act.At.Seconds(), FiredAtSec: now.Sub(a.start).Seconds(), Seconds: time.Since(began).Seconds()}
	text := strings.TrimSpace(string(out))
	r.Output = text[max(0, len(text)-ActionLimits.Output):]
	if err != nil {
		r.Error = err.Error()
	}
	a.mu.Lock()
	a.results = append(a.results, r)
	a.mu.Unlock()
}

// LoadgenArgs renders loadgen command lines from a case.
type LoadgenArgs struct{}

// MeasureDrainIdle is how long the measurer keeps reading after being stopped with no new records.
var MeasureDrainIdle = 5 * time.Second

// Produce returns the arguments of producer idx of N (rate split evenly, own stream prefix).
func (LoadgenArgs) Produce(bootstrap string, topics []string, l scenario.LoadSpec, idx int, interval time.Duration) []string {
	var steps []string
	for _, st := range l.Plan().Steps {
		steps = append(steps, fmt.Sprintf("%s:%s", strconv.FormatFloat(st.Rate/float64(l.Producers), 'f', -1, 64), st.Duration))
	}
	streams := max(l.Streams/l.Producers, len(topics), 1)
	args := []string{
		"--brokers", bootstrap, "--topic", strings.Join(topics, ","), "--steps", strings.Join(steps, ","),
		"--lanes", strconv.Itoa(l.Lanes), "--streams", strconv.Itoa(streams), "--stream-prefix", fmt.Sprintf("p%d", idx),
		"--keys", l.Keys, "--size", strconv.Itoa(l.Size), "--acks", l.Acks, "--linger", l.Linger.String(),
		"--compression", l.Codec, "--bad-ratio", strconv.FormatFloat(l.BadRatio, 'f', -1, 64), "--interval", interval.String(),
	}
	if l.BatchSize > 0 {
		args = append(args, "--batch-bytes", strconv.Itoa(l.BatchSize))
	}
	if l.Template != "" {
		args = append(args, "--template", l.Template)
	}
	return args
}

// Measure returns the arguments of the measurer.
func (LoadgenArgs) Measure(bootstrap string, topics []string, m scenario.MeasureSpec, interval time.Duration) []string {
	args := []string{
		"--brokers", bootstrap, "--topics", strings.Join(topics, ","), "--from", "end", "--seq-source", m.SeqSource,
		"--interval", interval.String(), "--drain-idle", MeasureDrainIdle.String(), "--track-by", m.TrackBy,
	}
	if m.ReadCommitted {
		args = append(args, "--read-committed")
	}
	return args
}

// HookRunner runs case hooks synchronously.
type HookRunner struct {
	Log func(string, ...any)
}

// Run executes commands in order and stops at the first failure.
func (h HookRunner) Run(ctx context.Context, phase string, commands []string) error {
	for _, c := range commands {
		h.Log("hook %s: %s", phase, c)
		ctx, cancel := context.WithTimeout(ctx, ActionLimits.Timeout)
		out, err := exec.CommandContext(ctx, "sh", "-c", c).CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("hook %s %q: %w: %s", phase, c, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
