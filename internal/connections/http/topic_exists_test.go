package connhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/pkg/kafka"
)

func TestTopicBrowserTopicExistsBacksTheDestinationTopicCheck(t *testing.T) {
	kc, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "orders"))
	if err != nil {
		t.Fatal(err)
	}
	defer kc.Close()
	h := newHarness(t)
	body := `{"name":"plain","envTag":"dev","bootstrapServers":["` + kc.ListenAddrs()[0] + `"],"security":{"mode":"plaintext"}}`
	resp, raw := h.do(t, http.MethodPost, "/api/v1/connections", body, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}
	var c connhttp.ConnectionDTO
	_ = json.Unmarshal(raw, &c)
	failures := connections.ClusterFailures{Classifier: kafka.DefaultFailureClassifier(), Hints: connections.DefaultHintCatalog()}
	browser := connections.NewTopicBrowser(connections.NewSpecResolver(h.repo, h.vault), kafka.DefaultClientFactoryRegistry(), connectionstest.MemRates{}, failures)
	for topic, want := range map[string]bool{"orders": true, "ord": false, "missing": false} {
		got, err := browser.TopicExists(context.Background(), c.ID.String(), topic)
		if err != nil || got != want {
			t.Fatalf("TopicExists(%q) = %v, %v; want %v", topic, got, err, want)
		}
	}
	if _, err := browser.TopicExists(context.Background(), "not-a-uuid", "orders"); err == nil {
		t.Fatal("an invalid connection id must be an error (the validator turns it into a warning)")
	}
}
