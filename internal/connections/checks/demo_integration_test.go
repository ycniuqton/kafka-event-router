package checks_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

func (e env) saveDemo(t *testing.T) connections.Connection {
	t.Helper()
	d := connections.DemoDraft{}.For(uuid.New())
	c, err := e.repo.Create(context.Background(), connections.Connection{
		TeamID: d.TeamID, Name: d.Name, EnvTag: d.EnvTag, BootstrapServers: d.BootstrapServers, Kind: d.Kind,
		Security: d.Security, AllowTopicCreate: d.AllowTopicCreate, Health: connections.Health{Status: connections.HealthUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { kafka.SharedDemoClusterPool().Release(kafka.DemoClusterKey(c.ID.String())) })
	return c
}

func TestDemoConnectionPassesFullChecklist(t *testing.T) {
	e := newEnv()
	c := e.saveDemo(t)
	topic := string(kafka.DemoOrderEvents)
	sink, saved := e.run(t, c.ID, connections.TestOptions{ReadTopic: topic, WriteTopic: topic})
	if !sink.Result.OK || saved.Health.Status != connections.HealthHealthy {
		t.Fatalf("result %+v events %+v", sink.Result, sink.Events)
	}
	for _, ev := range sink.Events {
		if ev.State == connections.StateSkipped || ev.State == connections.StateFailed {
			t.Fatalf("no step may be skipped or fail: %+v", ev)
		}
	}
}

func TestDemoTopicsAndPeek(t *testing.T) {
	e := newEnv()
	c := e.saveDemo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	topics, err := e.topics.List(ctx, c.ID, connections.TopicQuery{})
	if err != nil || len(topics) == 0 || topics[0].Name != string(kafka.DemoOrderEvents) {
		t.Fatalf("topics %+v %v", topics, err)
	}
	if _, err := e.creator.Create(ctx, c.ID, connections.TopicRequest{Name: "orders-enriched"}); err != nil {
		t.Fatalf("demo allows topic creation: %v", err)
	}
	cl, err := kafka.DefaultClientFactoryRegistry().NewClient(ctx, c.Spec(kafka.SecretBundle{}), kafka.PurposeSampler)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	marks, err := kafka.NewFranzMarksReader(cl.Raw()).Marks(ctx, []string{string(kafka.DemoOrderEvents)}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	windows := kafka.LatestOffsetPlanner{}.Plan(kafka.SamplingPlan{Count: 20}, marks)
	records, _, err := kafka.NewFranzPartitionSampler(cl.Raw()).Read(ctx, windows, kafka.NewSamplingBudget(20, 10<<20, time.Now().Add(5*time.Second)))
	if err != nil || len(records) != 20 {
		t.Fatalf("peek: %d %v", len(records), err)
	}
	for _, r := range records {
		var ev kafka.DemoOrderEvent
		if err := json.Unmarshal(r.Value, &ev); err != nil || ev.OrderID == "" || ev.EventType == "" {
			t.Fatalf("not an order event: %s", r.Value)
		}
	}
}
