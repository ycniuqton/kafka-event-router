package runner

import (
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/membudget"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// SourceFetch sizes the source consumer's fetch buffering from the worker budget (fetched, not yet polled data
// is not charged); an explicit fetch.max.bytes on the connection wins.
type SourceFetch struct {
	Budget *membudget.Budget
}

// Options returns the fetch caps of v's source consumer (none when the connection sets fetch.max.bytes).
func (f SourceFetch) Options(v CompiledVersion) []kgo.Opt {
	if _, set := v.Connections[v.Config.Source.ConnectionID].Properties[kafka.PropFetchMaxBytes]; set {
		return nil
	}
	l := membudget.OrUnlimited(f.Budget).FetchLimits()
	return []kgo.Opt{kgo.FetchMaxBytes(l.MaxBytes), kgo.FetchMaxPartitionBytes(l.PartitionMaxBytes)}
}

// MemoryAccount is the lane's view of the route's share of the worker memory budget.
type MemoryAccount interface {
	Charge(n int64)
	Release(n int64)
	Detaching() bool
}

// NoMemoryAccount accounts nothing (lanes built outside a runner, tests).
type NoMemoryAccount struct{}

func (NoMemoryAccount) Charge(int64)    {}
func (NoMemoryAccount) Release(int64)   {}
func (NoMemoryAccount) Detaching() bool { return false }

// TopicFetcher is the consumer surface the budget pauses: whole topics, so partitions assigned while the
// route is paused stay paused too. Topic pauses are independent of the gate's partition pauses.
type TopicFetcher interface {
	PauseFetchTopics(topics ...string) []string
	ResumeFetchTopics(topics ...string)
	GetConsumeTopics() []string
}

// SourcePauser pauses and resumes the source topics of one route for the memory budget.
type SourcePauser struct {
	client TopicFetcher
	topics []string
	mu     sync.Mutex
	paused []string
}

// NewSourcePauser pauses src.topics, or the topics the pattern currently matches.
func NewSourcePauser(client TopicFetcher, src config.Source) *SourcePauser {
	return &SourcePauser{client: client, topics: src.Topics}
}

// PauseFetch implements membudget.Pauser.
func (p *SourcePauser) PauseFetch() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = p.topics
	if len(p.paused) == 0 {
		p.paused = p.client.GetConsumeTopics()
	}
	if len(p.paused) > 0 {
		p.client.PauseFetchTopics(p.paused...)
	}
}

// ResumeFetch implements membudget.Pauser.
func (p *SourcePauser) ResumeFetch() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.paused) > 0 {
		p.client.ResumeFetchTopics(p.paused...)
	}
	p.paused = nil
}

// openAccount registers the runner with the worker budget (unlimited when the worker has none).
func (r *RouteRunner) openAccount(v CompiledVersion) *membudget.Account {
	return membudget.OrUnlimited(r.deps.Budget).Open(v.Ref.RouteID, NewSourcePauser(r.consumer.Client(), v.Config.Source))
}

// closeAccount returns the runner's remaining usage to the budget.
func (r *RouteRunner) closeAccount() {
	if r.account != nil {
		r.account.Close()
	}
}
