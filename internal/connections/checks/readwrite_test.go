package checks_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/connections/checks"
	"github.com/eventrouter/router/pkg/kafka"
)

func TestSamplerRecordReader(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(3, "orders", "quiet"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := kafka.DefaultClientFactoryRegistry().NewClient(ctx, kafka.ConnectionSpec{BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}, kafka.PurposeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for i := 0; i < 5; i++ {
		if err := client.Raw().ProduceSync(ctx, &kgo.Record{Topic: "orders", Value: []byte("x")}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
	for _, topic := range []string{"orders", "quiet", "orders"} {
		if err := (checks.SamplerRecordReader{}).ReadOne(ctx, client, kafka.TopicInfo{Name: topic, Partitions: 3}); err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
	}
	if groups, err := kadm.NewClient(client.Raw()).ListGroups(ctx); err != nil || len(groups) != 0 {
		t.Fatalf("read check must not create groups: %v %v", groups.Groups(), err)
	}
}
