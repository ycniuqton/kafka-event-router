package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/loadgen"
	"github.com/eventrouter/router/test/stress/kit/platform"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Env is what every case of a run shares.
type Env struct {
	Scenario     scenario.Scenario
	Target       scenario.Target
	Platform     *platform.Platform
	Kafka        KafkaAdmin
	Sampler      Sampling
	Launcher     Launcher
	Registry     *Registry
	Naming       Naming
	ConnectionID string
	Log          func(format string, args ...any)
}

// Provisioned is the set of routes deployed for a case (kept across cases of an incremental scenario).
type Provisioned struct {
	Topics   []platform.RouteTopics
	RouteIDs []string
	Deploy   []time.Duration
}

// CaseRunner runs one case: provision, measure, load, sample, drain, analyse.
type CaseRunner struct {
	env  *Env
	spec scenario.Case
	prov *Provisioned
}

// Run executes the case and returns its result (errors are recorded in the result).
func (r *CaseRunner) Run(ctx context.Context) CaseResult {
	res := CaseResult{Name: r.spec.Name, Spec: r.spec, StartedAt: time.Now()}
	hooks := HookRunner{Log: r.env.Log}
	defer func() {
		if err := hooks.Run(context.Background(), "after", r.spec.Hooks.After); err != nil {
			r.env.Log("%v", err)
		}
	}()
	setupStart := time.Now()
	if err := hooks.Run(ctx, "before", r.spec.Hooks.Before); err != nil {
		res.Error = err.Error()
		return res
	}
	if err := r.provision(ctx); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Setup = SetupResult{Routes: len(r.prov.RouteIDs), Seconds: time.Since(setupStart).Seconds(), Deploy: r.deployStats()}
	r.env.Sampler.Track(r.prov.RouteIDs, r.groups())
	tl, loop, err := r.drive(ctx)
	if err != nil {
		res.Error = err.Error()
	}
	res.Timeline = tl.Points
	if loop != nil {
		res.Produce, res.Measure, res.Actions = loop.produceSums, loop.measureSum, loop.Actions
	}
	res.Knee = knee.Detector{Criteria: r.env.Scenario.Criteria}.Analyze(tl.Steps(res.Produce))
	res.Delivery = DeliveryCheck{}.Verdict(res.Produce, res.Measure, r.copies())
	res.FinishedAt = time.Now()
	return res
}

func (r *CaseRunner) provision(ctx context.Context) error {
	have := len(r.prov.Topics)
	want := r.spec.Routes.Count
	if have > want {
		return fmt.Errorf("incremental scenario cannot shrink from %d to %d routes", have, want)
	}
	topics := r.env.Naming.Routes(r.spec.Name, r.spec.Routes.Topology, have, want-have)
	if err := r.createTopics(ctx, topics); err != nil {
		return err
	}
	ids, deploys, err := RouteDeployer{env: r.env, spec: r.spec.Routes}.DeployAll(ctx, topics)
	r.prov.Topics = append(r.prov.Topics, topics...)
	r.prov.RouteIDs = append(r.prov.RouteIDs, ids...)
	r.prov.Deploy = append(r.prov.Deploy, deploys...)
	return err
}

func (r *CaseRunner) createTopics(ctx context.Context, topics []platform.RouteTopics) error {
	var names []string
	seen := map[string]bool{}
	for _, t := range topics {
		for _, n := range []string{t.Input, t.Output, t.DLQ} {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
		r.env.Registry.Group(t.Group)
	}
	r.env.Registry.Topics(names...)
	return r.env.Kafka.EnsureTopics(ctx, r.spec.Partitions, names...)
}

// copies is how many tracked outputs each produced event must reach.
func (r *CaseRunner) copies() int {
	if r.spec.Measure.TrackBy == string(loadgen.TrackTopicStream) {
		return len(r.prov.Topics) / max(len(r.inputTopics()), 1)
	}
	return 1
}

func (r *CaseRunner) groups() []string {
	out := make([]string, len(r.prov.Topics))
	for i, t := range r.prov.Topics {
		out[i] = t.Group
	}
	return out
}

func (r *CaseRunner) deployStats() latency.Summary {
	h := latency.New()
	for _, d := range r.prov.Deploy {
		h.Record(d)
	}
	return h.Summary()
}

// drive starts the measurer and producers and samples until the load ends and the lag has drained.
func (r *CaseRunner) drive(ctx context.Context) (*Timeline, *SampleLoop, error) {
	tl := NewTimeline(time.Now())
	measure, err := r.startMeasure(ctx)
	if err != nil {
		return tl, nil, err
	}
	producers, err := r.startProducers(ctx)
	if err != nil {
		r.stopAll(measure, producers)
		return tl, nil, err
	}
	loop := &SampleLoop{env: r.env, spec: r.spec, tl: tl, producers: producers, measure: measure}
	loop.Run(ctx)
	return tl, loop, loop.err
}

func (r *CaseRunner) startMeasure(ctx context.Context) (*Loadgen, error) {
	if r.spec.Measure.Disabled {
		return nil, nil
	}
	var topics []string
	for _, t := range r.prov.Topics {
		topics = append(topics, t.Output)
		if r.spec.Measure.IncludeDLQ {
			topics = append(topics, t.DLQ)
		}
	}
	args := LoadgenArgs{}.Measure(r.env.Target.Kafka.LoadgenBootstrap, topics, r.spec.Measure, r.env.Scenario.SampleInterval)
	g, err := r.env.Launcher.Start(ctx, r.spec.Name+"-measure", "measure", args)
	if err != nil {
		return nil, err
	}
	if err := g.AwaitReady(ctx); err != nil {
		g.Stop()
		return nil, err
	}
	return g, nil
}

func (r *CaseRunner) startProducers(ctx context.Context) ([]*Loadgen, error) {
	inputs := r.inputTopics()
	var out []*Loadgen
	for i := 0; i < r.spec.Load.Producers; i++ {
		args := LoadgenArgs{}.Produce(r.env.Target.Kafka.LoadgenBootstrap, inputs, r.spec.Load, i, r.env.Scenario.SampleInterval)
		g, err := r.env.Launcher.Start(ctx, fmt.Sprintf("%s-produce-%d", r.spec.Name, i), "produce", args)
		if err != nil {
			return out, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *CaseRunner) inputTopics() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range r.prov.Topics {
		if !seen[t.Input] {
			seen[t.Input] = true
			out = append(out, t.Input)
		}
	}
	return out
}

func (r *CaseRunner) stopAll(measure *Loadgen, producers []*Loadgen) {
	for _, g := range append(producers, measure) {
		if g != nil {
			g.Stop()
			_ = g.Wait(LaunchLimits.StopGrace)
		}
	}
}

// RouteDeployer creates and deploys routes with bounded concurrency.
type RouteDeployer struct {
	env  *Env
	spec scenario.RouteSpec
}

// DeployAll creates, deploys and waits for every route; returns ids and per-route deploy→running latency.
func (d RouteDeployer) DeployAll(ctx context.Context, topics []platform.RouteTopics) ([]string, []time.Duration, error) {
	ids := make([]string, len(topics))
	lat := make([]time.Duration, len(topics))
	errs := make([]error, len(topics))
	sem := make(chan struct{}, max(d.spec.DeployBatch, 1))
	var wg sync.WaitGroup
	for i, t := range topics {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t platform.RouteTopics) {
			defer func() { <-sem; wg.Done() }()
			ids[i], lat[i], errs[i] = d.one(ctx, t)
		}(i, t)
	}
	wg.Wait()
	var okIDs []string
	var okLat []time.Duration
	var failed []string
	for i := range topics {
		if errs[i] != nil {
			failed = append(failed, errs[i].Error())
			continue
		}
		okIDs, okLat = append(okIDs, ids[i]), append(okLat, lat[i])
	}
	if len(failed) > 0 {
		return okIDs, okLat, fmt.Errorf("%d/%d routes failed: %s", len(failed), len(topics), strings.Join(failed[:min(3, len(failed))], "; "))
	}
	return okIDs, okLat, nil
}

func (d RouteDeployer) one(ctx context.Context, t platform.RouteTopics) (string, time.Duration, error) {
	draft, err := platform.DraftBuilder{Spec: d.spec, ConnectionID: d.env.ConnectionID}.Build(t)
	if err != nil {
		return "", 0, err
	}
	id, err := d.env.Platform.CreateRoute(ctx, t.Name, draft)
	if err != nil {
		return "", 0, err
	}
	d.env.Registry.Route(id)
	if d.spec.Replicas > 1 {
		if err := d.env.Platform.SetReplicas(ctx, id, d.spec.Replicas); err != nil {
			return "", 0, err
		}
	}
	start := time.Now()
	if err := d.env.Platform.Deploy(ctx, id); err != nil {
		return "", 0, err
	}
	if _, err := d.env.Platform.WaitRunning(ctx, id); err != nil {
		return "", 0, err
	}
	return id, time.Since(start), nil
}
