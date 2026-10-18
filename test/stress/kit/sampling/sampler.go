package sampling

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Metric is one worker metric the runner records; PerRoute metrics are summed over the run's routes only.
type Metric struct {
	Key      string
	Name     string
	PerRoute bool
}

// WorkerMetrics are the worker series sampled every interval.
var WorkerMetrics = []Metric{
	{Key: "consumed", Name: "router_events_consumed_total", PerRoute: true},
	{Key: "produced", Name: "router_events_produced_total", PerRoute: true},
	{Key: "dlq", Name: "router_events_dlq_total", PerRoute: true},
	{Key: "filtered", Name: "router_events_filtered_total", PerRoute: true},
	{Key: "lag", Name: "router_consumer_lag", PerRoute: true},
	{Key: "goroutines", Name: "go_goroutines"},
	{Key: "rssBytes", Name: "process_resident_memory_bytes"},
	{Key: "cpuSeconds", Name: "process_cpu_seconds_total"},
	{Key: "producerClients", Name: "router_worker_producer_clients"},
}

// ProcStat holds the /proc/<pid>/stat layout and kernel units.
var ProcStat = struct {
	UtimeField, StimeField, ThreadsField, RSSField int
	ClockTicks, PageBytes                          float64
}{UtimeField: 11, StimeField: 12, ThreadsField: 17, RSSField: 21, ClockTicks: 100, PageBytes: 4096}

// WorkerSnapshot is one worker's metrics at a sample instant.
type WorkerSnapshot struct {
	Name   string             `json:"name"`
	Values map[string]float64 `json:"values"`
	Err    string             `json:"error,omitempty"`
}

// ProcessSnapshot is one process's /proc stats.
type ProcessSnapshot struct {
	Name       string  `json:"name"`
	PID        int     `json:"pid"`
	CPUSeconds float64 `json:"cpuSeconds"`
	RSSBytes   float64 `json:"rssBytes"`
	Threads    int     `json:"threads"`
	Err        string  `json:"error,omitempty"`
}

// Snapshot is everything sampled at one instant.
type Snapshot struct {
	At        time.Time         `json:"at"`
	Workers   []WorkerSnapshot  `json:"workers"`
	Processes []ProcessSnapshot `json:"processes"`
	GroupLag  int64             `json:"groupLag"`
	LagErr    string            `json:"lagError,omitempty"`
}

// Sampler samples the configured sources.
type Sampler struct {
	workers   []namedFetcher
	processes []processSource
	admin     *kafkaops.Admin
	mu        sync.Mutex
	routeIDs  map[string]bool
	groups    []string
}

type namedFetcher struct {
	name string
	f    Fetcher
}

type processSource struct {
	spec   scenario.ProcessSpec
	pid    Fetcher
	reader ProcReader
}

// NewSampler builds fetchers for a target; admin is used for consumer-group lag.
func NewSampler(t scenario.Target, admin *kafkaops.Admin) (*Sampler, error) {
	s := &Sampler{admin: admin, routeIDs: map[string]bool{}}
	for _, m := range t.Metrics {
		f, err := FetcherManager{}.For(m.Source)
		if err != nil {
			return nil, err
		}
		s.workers = append(s.workers, namedFetcher{name: m.Name, f: f})
	}
	for _, p := range t.Processes {
		f, err := FetcherManager{}.For(p.PID)
		if err != nil {
			return nil, err
		}
		s.processes = append(s.processes, processSource{spec: p, pid: f, reader: ProcReader{Prefix: p.Reader}})
	}
	return s, nil
}

// Track sets the routes (worker metric filter) and consumer groups (lag) of the running case.
func (s *Sampler) Track(routeIDs, groups []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeIDs = map[string]bool{}
	for _, id := range routeIDs {
		s.routeIDs[id] = true
	}
	s.groups = append([]string{}, groups...)
}

// Sample takes one snapshot (sources sampled concurrently; failures are recorded, not fatal).
func (s *Sampler) Sample(ctx context.Context) Snapshot {
	s.mu.Lock()
	routes, groups := s.routeIDs, s.groups
	s.mu.Unlock()
	snap := Snapshot{At: time.Now(), Workers: make([]WorkerSnapshot, len(s.workers)), Processes: make([]ProcessSnapshot, len(s.processes))}
	var wg sync.WaitGroup
	for i := range s.workers {
		wg.Add(1)
		go func(i int) { defer wg.Done(); snap.Workers[i] = s.worker(ctx, s.workers[i], routes) }(i)
	}
	for i := range s.processes {
		wg.Add(1)
		go func(i int) { defer wg.Done(); snap.Processes[i] = s.process(ctx, s.processes[i]) }(i)
	}
	snap.GroupLag, snap.LagErr = s.lag(ctx, groups)
	wg.Wait()
	return snap
}

// PIDs resolves the current pid of every configured process (for CPU pinning).
func (s *Sampler) PIDs(ctx context.Context) map[string]int {
	out := map[string]int{}
	for _, p := range s.processes {
		if raw, err := p.pid.Fetch(ctx); err == nil {
			if pid, err := (PIDResolver{}).Parse(raw); err == nil {
				out[p.spec.Name] = pid
			}
		}
	}
	return out
}

func (s *Sampler) worker(ctx context.Context, nf namedFetcher, routes map[string]bool) WorkerSnapshot {
	ws := WorkerSnapshot{Name: nf.name, Values: map[string]float64{}}
	raw, err := nf.f.Fetch(ctx)
	if err != nil {
		ws.Err = err.Error()
		return ws
	}
	series := PromText{}.Parse(raw)
	for _, m := range WorkerMetrics {
		sel := Select{Name: m.Name}
		if m.PerRoute {
			sel.Label, sel.Values = "route_id", routes
		}
		if v, ok := sel.Sum(series); ok {
			ws.Values[m.Key] = v
		}
	}
	return ws
}

func (s *Sampler) process(ctx context.Context, src processSource) ProcessSnapshot {
	ps := ProcessSnapshot{Name: src.spec.Name}
	raw, err := src.pid.Fetch(ctx)
	if err == nil {
		ps.PID, err = PIDResolver{}.Parse(raw)
	}
	if err == nil {
		raw, err = src.reader.Read(ctx, ps.PID, "stat")
	}
	if err != nil {
		ps.Err = err.Error()
		return ps
	}
	return ProcStatParser{}.Fill(ps, raw)
}

func (s *Sampler) lag(ctx context.Context, groups []string) (int64, string) {
	if s.admin == nil || len(groups) == 0 {
		return 0, ""
	}
	lags, err := s.admin.GroupLag(ctx, groups...)
	if err != nil {
		return 0, err.Error()
	}
	var total int64
	for _, l := range lags {
		total += l
	}
	return total, ""
}

// ProcStatParser decodes /proc/<pid>/stat.
type ProcStatParser struct{}

// Fill sets CPU seconds, RSS and threads from raw stat content.
func (ProcStatParser) Fill(ps ProcessSnapshot, raw []byte) ProcessSnapshot {
	text := string(raw)
	f := strings.Fields(text[strings.LastIndexByte(text, ')')+1:])
	if len(f) <= ProcStat.RSSField {
		ps.Err = "short /proc stat"
		return ps
	}
	num := func(i int) float64 { v, _ := strconv.ParseFloat(f[i], 64); return v }
	ps.CPUSeconds = (num(ProcStat.UtimeField) + num(ProcStat.StimeField)) / ProcStat.ClockTicks
	ps.RSSBytes = num(ProcStat.RSSField) * ProcStat.PageBytes
	ps.Threads = int(num(ProcStat.ThreadsField))
	return ps
}
