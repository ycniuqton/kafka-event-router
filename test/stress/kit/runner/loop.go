package runner

import (
	"context"
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/scenario"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// sourced is one report line tagged with the loadgen it came from; closed marks that loadgen's exit.
type sourced struct {
	source string
	line   stream.Line
	closed bool
}

// SampleLoop multiplexes loadgen reports, interval sampling, timed actions and the knee stop rule of one case.
type SampleLoop struct {
	env         *Env
	spec        scenario.Case
	tl          *Timeline
	producers   []*Loadgen
	measure     *Loadgen
	events      chan sourced
	openProd    int
	measureOpen bool
	stopped     bool
	produceSums []stream.ProduceSummary
	measureSum  *stream.MeasureSummary
	Actions     []ActionResult
	err         error
}

// Run drives the case until producers finished, lag drained (or timed out) and the measurer reported.
func (l *SampleLoop) Run(ctx context.Context) {
	l.fanIn()
	tick := time.NewTicker(l.env.Scenario.SampleInterval)
	defer tick.Stop()
	actions := NewActionScheduler(l.spec.Actions, time.Now(), l.env.Log)
	for l.openProd > 0 {
		select {
		case ev := <-l.events:
			l.onEvent(ev)
		case <-tick.C:
			l.tl.OnSnapshot(l.env.Sampler.Sample(ctx))
			actions.Fire(ctx, time.Now())
		case <-ctx.Done():
			l.stopProducers("interrupted")
			l.err = ctx.Err()
		}
	}
	l.drain(ctx, tick)
	l.finishMeasure()
	l.Actions = actions.Wait()
}

func (l *SampleLoop) fanIn() {
	l.events = make(chan sourced, 1024)
	all := append([]*Loadgen{}, l.producers...)
	l.openProd = len(l.producers)
	if l.measure != nil {
		all = append(all, l.measure)
		l.measureOpen = true
	}
	for _, g := range all {
		go func(g *Loadgen) {
			for line := range g.Lines {
				l.events <- sourced{source: g.Name, line: line}
			}
			l.events <- sourced{source: g.Name, closed: true}
		}(g)
	}
}

func (l *SampleLoop) onEvent(ev sourced) {
	isMeasure := l.measure != nil && ev.source == l.measure.Name
	switch {
	case ev.closed && isMeasure:
		l.measureOpen = false
	case ev.closed:
		l.openProd--
		l.tl.OnClosed(ev.source)
	case ev.line.Kind == stream.KindSummary && ev.line.PSum != nil:
		l.produceSums = append(l.produceSums, *ev.line.PSum)
	case ev.line.Kind == stream.KindSummary && ev.line.MSum != nil:
		l.measureSum = ev.line.MSum
	default:
		before := l.tl.CurrentStep()
		l.tl.OnLine(ev.source, ev.line)
		if l.tl.CurrentStep() > before && before >= 0 {
			l.checkKnee(before)
		}
	}
}

func (l *SampleLoop) checkKnee(step int) {
	if !l.env.Scenario.StopOnKnee || l.stopped {
		return
	}
	res := knee.Detector{Criteria: l.env.Scenario.Criteria}.Evaluate(l.tl.Provisional(step, time.Now()))
	l.env.Log("case %s step %d (%.0f ev/s): %s (lag slope %.1f/s, p99 %.1f ms)", l.spec.Name, step, res.TargetRate, res.Verdict, res.LagSlope, res.Latency.P99Ms)
	if res.Verdict == knee.VerdictLagGrowing || res.Verdict == knee.VerdictLatency {
		l.stopProducers("knee: " + string(res.Verdict))
	}
}

func (l *SampleLoop) stopProducers(reason string) {
	if l.stopped {
		return
	}
	l.stopped = true
	l.env.Log("case %s: stopping load (%s)", l.spec.Name, reason)
	for _, g := range l.producers {
		g.Stop()
	}
}

func (l *SampleLoop) drain(ctx context.Context, tick *time.Ticker) {
	deadline := time.Now().Add(l.spec.Drain.Timeout)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		select {
		case ev := <-l.events:
			l.onEvent(ev)
		case <-tick.C:
			p := l.tl.OnSnapshot(l.env.Sampler.Sample(ctx))
			if p.GroupLag == 0 && p.WorkerLag == 0 {
				return
			}
		}
	}
	l.env.Log("case %s: lag not drained within %s", l.spec.Name, l.spec.Drain.Timeout)
}

func (l *SampleLoop) finishMeasure() {
	if l.measure == nil {
		return
	}
	l.measure.Stop()
	grace := time.After(LaunchLimits.StopGrace)
	for l.measureOpen {
		select {
		case ev := <-l.events:
			l.onEvent(ev)
		case <-grace:
			l.measureOpen = false
		}
	}
	_ = l.measure.Wait(time.Second)
	for _, g := range l.producers {
		_ = g.Wait(time.Second)
	}
}
