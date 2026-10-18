package runner

import (
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/sampling"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// Point is one sampled instant of a case, with rates derived from the previous snapshot.
type Point struct {
	At              time.Time               `json:"at"`
	ElapsedSec      float64                 `json:"elapsedSec"`
	Step            int                     `json:"step"`
	OfferedRate     float64                 `json:"offeredRate"`
	ConsumedRate    float64                 `json:"consumedRate"`
	ProducedRate    float64                 `json:"producedRate"`
	DLQRate         float64                 `json:"dlqRate"`
	DeliveredRate   float64                 `json:"deliveredRate"`
	GroupLag        int64                   `json:"groupLag"`
	WorkerLag       float64                 `json:"workerLag"`
	P99Ms           float64                 `json:"p99Ms"`
	Goroutines      float64                 `json:"goroutines"`
	WorkerRSSMB     float64                 `json:"workerRssMb"`
	WorkerCPUCores  float64                 `json:"workerCpuCores"`
	ProducerClients float64                 `json:"producerClients,omitempty"`
	Processes       map[string]ProcessPoint `json:"processes,omitempty"`
	Errors          []string                `json:"errors,omitempty"`
}

// ProcessPoint is a process's resource use at one instant.
type ProcessPoint struct {
	PID      int     `json:"pid"`
	CPUCores float64 `json:"cpuCores"`
	RSSMB    float64 `json:"rssMb"`
	Threads  int     `json:"threads"`
}

// Units for reported sizes.
var Units = struct{ MB float64 }{MB: 1 << 20}

// Timeline accumulates points, step boundaries and measure windows of one case.
type Timeline struct {
	start     time.Time
	prev      *sampling.Snapshot
	Points    []Point
	steps     []knee.Step
	producers map[string]float64
	lastP99   float64
	windows   []stream.Line
}

// NewTimeline starts a timeline at now.
func NewTimeline(now time.Time) *Timeline {
	return &Timeline{start: now, producers: map[string]float64{}}
}

// OnLine folds one loadgen report line into the timeline.
func (t *Timeline) OnLine(source string, l stream.Line) {
	switch {
	case l.Kind == stream.KindStep && l.Step != nil:
		t.onStep(*l.Step)
	case l.Kind == stream.KindWindow && l.Produce != nil:
		t.producers[source] = l.Produce.AckedRate
	case l.Kind == stream.KindWindow && l.Measure != nil:
		t.windows = append(t.windows, l)
		t.lastP99 = l.Measure.Latency.P99Ms
	}
}

func (t *Timeline) onStep(s stream.StepReport) {
	for len(t.steps) <= s.Index {
		t.steps = append(t.steps, knee.Step{Index: len(t.steps), Latency: latency.New()})
	}
	st := &t.steps[s.Index]
	if st.Start.IsZero() || time.Unix(0, s.StartNs).Before(st.Start) {
		st.Start = time.Unix(0, s.StartNs)
	}
	st.TargetRate += s.TargetRate
}

// OnClosed forgets the offered rate of a loadgen that exited.
func (t *Timeline) OnClosed(source string) {
	delete(t.producers, source)
}

// CurrentStep is the index of the latest started step (-1 before the first).
func (t *Timeline) CurrentStep() int {
	return len(t.steps) - 1
}

// OnSnapshot adds a sampled point.
func (t *Timeline) OnSnapshot(s sampling.Snapshot) Point {
	p := Point{At: s.At, ElapsedSec: s.At.Sub(t.start).Seconds(), Step: t.CurrentStep(), GroupLag: s.GroupLag, P99Ms: t.lastP99}
	for _, r := range t.producers {
		p.OfferedRate += r
	}
	if s.LagErr != "" {
		p.Errors = append(p.Errors, "lag: "+s.LagErr)
	}
	PointBuilder{}.workers(&p, s, t.prev)
	PointBuilder{}.processes(&p, s, t.prev)
	t.prev = &s
	t.Points = append(t.Points, p)
	if p.Step >= 0 {
		st := &t.steps[p.Step]
		st.Samples = append(st.Samples, knee.Sample{At: s.At, Lag: PointBuilder{}.lag(p)})
	}
	return p
}

// Steps closes the step list with produce summaries and assigns measure windows to steps by time.
func (t *Timeline) Steps(summaries []stream.ProduceSummary) []knee.Step {
	for _, sum := range summaries {
		for _, sr := range sum.Steps {
			if sr.Index < len(t.steps) {
				t.steps[sr.Index].AchievedRate += sr.AchievedRate
				t.steps[sr.Index].End = t.later(t.steps[sr.Index].End, time.Unix(0, sr.EndNs))
			}
		}
	}
	for _, w := range t.windows {
		if i := t.stepAt(time.Unix(0, w.AtNs)); i >= 0 {
			t.steps[i].Latency.MergeSparse(w.Measure.Histogram)
		}
	}
	return t.steps
}

// Provisional returns step i as observed so far (ending at end), for the online knee stop rule.
func (t *Timeline) Provisional(i int, end time.Time) knee.Step {
	st := t.steps[i]
	st.End = end
	st.Latency = latency.New()
	var offered float64
	var n int
	for _, p := range t.Points {
		if p.Step == i && p.OfferedRate > 0 {
			offered += p.OfferedRate
			n++
		}
	}
	if n > 0 {
		st.AchievedRate = offered / float64(n)
	}
	for _, w := range t.windows {
		at := time.Unix(0, w.AtNs)
		if !at.Before(st.Start) && !at.After(end) {
			st.Latency.MergeSparse(w.Measure.Histogram)
		}
	}
	return st
}

func (t *Timeline) stepAt(at time.Time) int {
	for i := len(t.steps) - 1; i >= 0; i-- {
		st := t.steps[i]
		if !at.Before(st.Start) && (st.End.IsZero() || !at.After(st.End)) {
			return i
		}
	}
	return -1
}

func (*Timeline) later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// PointBuilder derives point fields from consecutive snapshots.
type PointBuilder struct{}

func (PointBuilder) lag(p Point) float64 {
	if p.GroupLag > 0 || p.WorkerLag == 0 {
		return float64(p.GroupLag)
	}
	return p.WorkerLag
}

func (b PointBuilder) workers(p *Point, s sampling.Snapshot, prev *sampling.Snapshot) {
	dt := b.dt(s, prev)
	for i, w := range s.Workers {
		if w.Err != "" {
			p.Errors = append(p.Errors, w.Name+": "+w.Err)
			continue
		}
		p.WorkerLag += w.Values["lag"]
		p.Goroutines += w.Values["goroutines"]
		p.WorkerRSSMB += w.Values["rssBytes"] / Units.MB
		p.ProducerClients += w.Values["producerClients"]
		if prev == nil || dt <= 0 || i >= len(prev.Workers) {
			continue
		}
		old := prev.Workers[i].Values
		p.ConsumedRate += b.rate(w.Values["consumed"], old["consumed"], dt)
		p.ProducedRate += b.rate(w.Values["produced"], old["produced"], dt)
		p.DLQRate += b.rate(w.Values["dlq"], old["dlq"], dt)
		p.WorkerCPUCores += b.rate(w.Values["cpuSeconds"], old["cpuSeconds"], dt)
	}
	p.DeliveredRate = p.ProducedRate + p.DLQRate
}

func (b PointBuilder) processes(p *Point, s sampling.Snapshot, prev *sampling.Snapshot) {
	dt := b.dt(s, prev)
	p.Processes = map[string]ProcessPoint{}
	for i, ps := range s.Processes {
		if ps.Err != "" {
			p.Errors = append(p.Errors, ps.Name+": "+ps.Err)
			continue
		}
		pp := ProcessPoint{PID: ps.PID, RSSMB: ps.RSSBytes / Units.MB, Threads: ps.Threads}
		if prev != nil && dt > 0 && i < len(prev.Processes) && prev.Processes[i].PID == ps.PID {
			pp.CPUCores = b.rate(ps.CPUSeconds, prev.Processes[i].CPUSeconds, dt)
		}
		p.Processes[ps.Name] = pp
	}
}

func (PointBuilder) dt(s sampling.Snapshot, prev *sampling.Snapshot) float64 {
	if prev == nil {
		return 0
	}
	return s.At.Sub(prev.At).Seconds()
}

func (PointBuilder) rate(now, before, dt float64) float64 {
	if now < before {
		return 0
	}
	return (now - before) / dt
}
