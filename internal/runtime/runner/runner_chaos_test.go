// Chaos and outage integration tests (RR-37, RR-58 Kafka half, RR-61) on an in-process kfake cluster.
// Toxiproxy is replaced by faultListener (broker stop/start for every client) and faultNetwork
// (one worker's network partition or kill); every key is counted, duplicates are reported.
package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/kafka"
)

// chaosTimings are the group timings that make a killed member expire within seconds.
var chaosTimings = struct {
	MinSession, Session, Heartbeat, Outage, Settle, Interval time.Duration
	Partitions, Batches, BatchSize                           int
}{MinSession: time.Second, Session: 3 * time.Second, Heartbeat: 300 * time.Millisecond, Outage: 3 * time.Second,
	Settle: 500 * time.Millisecond, Interval: 25 * time.Millisecond, Partitions: 4, Batches: 120, BatchSize: 25}

// faultListener is a broker listener that can refuse and sever every connection (broker stop/start).
type faultListener struct {
	net.Listener
	mu    sync.Mutex
	down  bool
	conns map[net.Conn]struct{}
}

func (l *faultListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.admit(conn) {
			return conn, nil
		}
		_ = conn.Close()
	}
}

func (l *faultListener) admit(conn net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.down {
		return false
	}
	l.conns[conn] = struct{}{}
	return true
}

// Stop severs open connections and refuses new ones until Start.
func (l *faultListener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.down = true
	for c := range l.conns {
		_ = c.Close()
	}
	l.conns = map[net.Conn]struct{}{}
}

// Start accepts connections again.
func (l *faultListener) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.down = false
}

// faultNetwork is one worker's network: Cut severs and refuses its connections until Heal.
type faultNetwork struct {
	mu    sync.Mutex
	cut   bool
	conns map[net.Conn]struct{}
}

func newFaultNetwork() *faultNetwork { return &faultNetwork{conns: map[net.Conn]struct{}{}} }

func (n *faultNetwork) Dial(ctx context.Context, network, host string) (net.Conn, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cut {
		return nil, fmt.Errorf("chaos: network of worker is cut")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, host)
	if err == nil {
		n.conns[conn] = struct{}{}
	}
	return conn, err
}

func (n *faultNetwork) Cut() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cut = true
	for c := range n.conns {
		_ = c.Close()
	}
	n.conns = map[net.Conn]struct{}{}
}

func (n *faultNetwork) Heal() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cut = false
}

// chaosOpener opens franz clients through one worker's faultNetwork with short group timings.
type chaosOpener struct {
	clients.FranzOpener
	network *faultNetwork
}

func (o chaosOpener) Open(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	opts := append(extra, kgo.Dialer(o.network.Dial))
	if purpose == kafka.PurposeConsumer {
		opts = append(opts, kgo.SessionTimeout(chaosTimings.Session), kgo.HeartbeatInterval(chaosTimings.Heartbeat))
	}
	return o.FranzOpener.Open(ctx, spec, purpose, opts...)
}

// chaosWorker is one simulated worker process: its own runner manager and network.
type chaosWorker struct {
	manager *runner.Manager
	network *faultNetwork
}

func newChaosWorker(name string) chaosWorker {
	network := newFaultNetwork()
	opener := clients.NewOpenerManager(chaosOpener{FranzOpener: clients.NewFranzOpener(), network: network})
	return chaosWorker{manager: newManagerWith(&haltRecorder{}, opener, name), network: network}
}

// chaosCluster is a kfake cluster whose broker listener can be stopped and started.
type chaosCluster struct {
	*kfake.Cluster
	listener *faultListener
}

func newChaosCluster(t *testing.T) chaosCluster {
	t.Helper()
	fl := &faultListener{conns: map[net.Conn]struct{}{}}
	listen := func(network, address string) (net.Listener, error) {
		ln, err := net.Listen(network, address)
		fl.Listener = ln
		return fl, err
	}
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(int32(chaosTimings.Partitions), "in", "out"),
		kfake.GroupMinSessionTimeout(chaosTimings.MinSession), kfake.ListenFn(listen))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return chaosCluster{Cluster: c, listener: fl}
}

// keyedLoad produces uniquely keyed events that all pass the route filter.
type keyedLoad struct{ c *kfake.Cluster }

func (l keyedLoad) Produce(t *testing.T, prefix string, n int) {
	t.Helper()
	recs := make([]*kgo.Record, 0, n)
	for i := 0; i < n; i++ {
		v, _ := json.Marshal(map[string]any{"keep": true, "n": i})
		recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("%s-%d", prefix, i)), Value: v})
	}
	deadline := time.Now().Add(30 * time.Second)
	err := l.attempt(recs)
	for err != nil && time.Now().Before(deadline) {
		time.Sleep(chaosTimings.Interval)
		err = l.attempt(recs)
	}
	if err != nil {
		t.Error(err)
	}
}

// attempt produces recs once with a fresh client (a broker outage can fail producer-id init for good).
func (l keyedLoad) attempt(recs []*kgo.Record) error {
	cl, err := kgo.NewClient(kgo.SeedBrokers(l.c.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		return err
	}
	defer cl.Close()
	for _, r := range recs {
		r.Topic = ""
	}
	return cl.ProduceSync(context.Background(), recs...).FirstErr()
}

// Stream produces batches of n keyed events every interval in the background; done closes after the last batch.
func (l keyedLoad) Stream(t *testing.T, prefix string, batches, n int) (total int, done <-chan struct{}) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		for b := 0; b < batches; b++ {
			l.Produce(t, fmt.Sprintf("%s%d", prefix, b), n)
			time.Sleep(chaosTimings.Interval)
		}
	}()
	return batches * n, ch
}

// outputAudit reads every record of the destination topic and counts deliveries per key.
type outputAudit struct{ c *kfake.Cluster }

func (a outputAudit) Keys(t *testing.T) map[string]int {
	t.Helper()
	cl := mustClient(t, a.c)
	ends, err := kadm.NewClient(cl).ListEndOffsets(context.Background(), "out")
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32]kgo.Offset{}
	remaining := map[int32]int64{}
	ends.Each(func(o kadm.ListedOffset) {
		want[o.Partition] = kgo.NewOffset().AtStart()
		if o.Offset > 0 {
			remaining[o.Partition] = o.Offset
		}
	})
	return a.read(t, map[string]map[int32]kgo.Offset{"out": want}, remaining)
}

func (a outputAudit) read(t *testing.T, parts map[string]map[int32]kgo.Offset, remaining map[int32]int64) map[string]int {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(a.c.ListenAddrs()...), kgo.ConsumePartitions(parts))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	keys := map[string]int{}
	for len(remaining) > 0 && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			keys[string(r.Key)]++
			if r.Offset+1 >= remaining[r.Partition] {
				delete(remaining, r.Partition)
			}
		})
	}
	return keys
}

// Verify asserts every produced key was delivered at least once and logs duplicates.
func (a outputAudit) Verify(t *testing.T, produced int) {
	t.Helper()
	keys := a.Keys(t)
	duplicates := 0
	for _, n := range keys {
		duplicates += n - 1
	}
	if len(keys) != produced {
		t.Fatalf("delivered %d distinct keys of %d produced: data loss", len(keys), produced)
	}
	t.Logf("delivered %d keys, %d duplicates (at-least-once window)", len(keys), duplicates)
}

// commitWatch samples that the group never commits past what reached the destination.
type commitWatch struct {
	c        *kfake.Cluster
	group    string
	violated chan string
	stop     chan struct{}
	done     chan struct{}
}

func watchCommits(t *testing.T, c *kfake.Cluster) *commitWatch {
	w := &commitWatch{c: c, group: "router.r1", violated: make(chan string, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go w.run(t)
	return w
}

func (w *commitWatch) run(t *testing.T) {
	defer close(w.done)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-tick.C:
			w.sample(t)
		}
	}
}

func (w *commitWatch) sample(t *testing.T) {
	done := committed(t, w.c, w.group)
	outputs := countRecords(t, w.c, "out")
	if outputs >= 0 && done > int64(outputs) {
		select {
		case w.violated <- fmt.Sprintf("committed %d but only %d outputs durable", done, outputs):
		default:
		}
	}
}

// Assert stops sampling and fails on any commit past an undelivered record.
func (w *commitWatch) Assert(t *testing.T) {
	close(w.stop)
	<-w.done
	select {
	case msg := <-w.violated:
		t.Fatal(msg)
	default:
	}
}

func TestChaosBrokerStopStartLosesNothingAndNeverCommitsPastFailure(t *testing.T) {
	c := newChaosCluster(t)
	w := newChaosWorker("w1")
	if err := w.manager.Start(context.Background(), compiled(t, c.Cluster, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer w.manager.StopAll(context.Background())
	watch := watchCommits(t, c.Cluster)
	total, done := keyedLoad{c: c.Cluster}.Stream(t, "k", chaosTimings.Batches, chaosTimings.BatchSize)
	eventually(t, "first deliveries", func() bool { return countRecords(t, c.Cluster, "out") > 0 })
	c.listener.Stop()
	time.Sleep(chaosTimings.Outage)
	c.listener.Start()
	<-done
	eventually(t, "all commits", func() bool { return committed(t, c.Cluster, "router.r1") == int64(total) })
	watch.Assert(t)
	outputAudit{c: c.Cluster}.Verify(t, total)
}

func TestChaosWorkerNetworkPartitionHealsWithoutLoss(t *testing.T) {
	c := newChaosCluster(t)
	w := newChaosWorker("w1")
	if err := w.manager.Start(context.Background(), compiled(t, c.Cluster, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer w.manager.StopAll(context.Background())
	total, done := keyedLoad{c: c.Cluster}.Stream(t, "k", chaosTimings.Batches, chaosTimings.BatchSize)
	eventually(t, "first deliveries", func() bool { return countRecords(t, c.Cluster, "out") > 0 })
	w.network.Cut()
	time.Sleep(chaosTimings.Outage)
	w.network.Heal()
	<-done
	eventually(t, "all commits", func() bool { return committed(t, c.Cluster, "router.r1") == int64(total) })
	outputAudit{c: c.Cluster}.Verify(t, total)
}

func TestChaosWorkerKillRebalancesToSurvivorWithoutLoss(t *testing.T) {
	c := newChaosCluster(t)
	victim, survivor := newChaosWorker("w1"), newChaosWorker("w2")
	v := compiled(t, c.Cluster, "dlq")
	for _, w := range []chaosWorker{victim, survivor} {
		if err := w.manager.Start(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	}
	defer survivor.manager.StopAll(context.Background())
	adm := kadm.NewClient(mustClient(t, c.Cluster))
	eventually(t, "two group members", func() bool { return groupSize(t, adm) == 2 })
	total, done := keyedLoad{c: c.Cluster}.Stream(t, "k", chaosTimings.Batches, chaosTimings.BatchSize)
	eventually(t, "first deliveries", func() bool { return countRecords(t, c.Cluster, "out") > 0 })
	victim.network.Cut()
	eventually(t, "victim expired from the group", func() bool { return groupSize(t, adm) == 1 })
	<-done
	eventually(t, "survivor commits everything", func() bool { return committed(t, c.Cluster, "router.r1") == int64(total) })
	time.Sleep(chaosTimings.Settle)
	outputAudit{c: c.Cluster}.Verify(t, total)
	stopQuietly(victim)
}

func groupSize(t *testing.T, adm *kadm.Client) int {
	t.Helper()
	groups, err := adm.DescribeGroups(context.Background(), "router.r1")
	if err != nil {
		return -1
	}
	return len(groups["router.r1"].Members)
}

// stopQuietly releases a killed worker's goroutines without waiting on its unreachable broker.
func stopQuietly(w chaosWorker) {
	ctx, cancel := context.WithTimeout(context.Background(), chaosTimings.Settle)
	defer cancel()
	w.manager.StopAll(ctx)
}
