package runner_test

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kadm"
)

const createTopicRouteJSON = `{"apiVersion":"router/v1","name":"orders","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"skip"},
"branches":[{"id":"main","destination":{"connectionId":"c1","topic":"fresh","createTopic":{"partitions":3}}}]}`

func TestRunnerCreatesMissingDestinationTopicWhenAsked(t *testing.T) {
	c := cluster(t)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), compiledJSON(t, compiled(t, c, "skip"), createTopicRouteJSON, 1)); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	details, err := kadm.NewClient(mustClient(t, c)).ListTopics(context.Background(), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := details["fresh"]; !ok || d.Err != nil || len(d.Partitions) != 3 {
		t.Fatalf("destination topic not created with 3 partitions: %+v", details)
	}
	produce(t, c, `{"id":1}`)
	eventually(t, "output on the created topic", func() bool { return countRecords(t, c, "fresh") == 1 })
}
