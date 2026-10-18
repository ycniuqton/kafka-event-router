package dlq

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/config"
)

func exactlyOnceConfig() string {
	return strings.Replace(e2eRouteConfig, `"errorPolicy"`, `"delivery": {"mode": "exactly_once"}, "errorPolicy"`, 1)
}

func TestRetryOnExactlyOnceRouteIsFlaggedAndNonTransactional(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	produceValues(t, opener, []string{`{"amount":200,"cust_id":1}`, `{"amount":300,"cust_id":2}`})
	m := e2eManagerWith(t, opener, exactlyOnceConfig())
	ctx := context.Background()
	page, err := m.List(ctx, "r1", Query{})
	if err != nil || len(page.Items) != 2 || page.Items[0].Delivery != config.DeliveryModeExactlyOnce {
		t.Fatalf("list %v %+v", err, page.Items)
	}
	report, err := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r1", Selection{Match: &Query{}})
	if err != nil || report.Counts()[OutcomeSent] != 2 {
		t.Fatalf("retry %v %+v", err, report)
	}
	if !report.Delivery.DeliveredAtLeastOnce || report.Delivery.Mode != config.DeliveryModeExactlyOnce {
		t.Fatalf("delivery notice %+v", report.Delivery)
	}
	assertCommittedNonTransactional(t, opener, 2)
}

func TestRetryOnAtLeastOnceRouteIsNotFlagged(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	produceValues(t, opener, []string{`{"amount":200,"cust_id":1}`})
	m := e2eManager(t, opener)
	report, err := m.Act(context.Background(), ActionRetry, Actor{UserID: "u1"}, "r1", Selection{Match: &Query{}})
	if err != nil || report.Delivery.DeliveredAtLeastOnce || report.Delivery.Mode != config.DeliveryModeAtLeastOnce {
		t.Fatalf("retry %v %+v", err, report.Delivery)
	}
}

func TestDeliveryModeOf(t *testing.T) {
	route := RouteInfo{ID: "r1", ActiveVersionID: "v2"}
	versions := []VersionConfig{{ID: "v1", Config: []byte(e2eRouteConfig)}, {ID: "v2", Config: []byte(exactlyOnceConfig())}}
	if got := (DeliveryModeOf{}).Active(route, versions); got != config.DeliveryModeExactlyOnce {
		t.Fatalf("active %q", got)
	}
	if got := (DeliveryModeOf{}).Active(RouteInfo{ID: "r1"}, versions); got != "" {
		t.Fatalf("undeployed %q", got)
	}
	if got := (DeliveryModeOf{}).Config([]byte(`{bad`)); got != config.DeliveryModeAtLeastOnce {
		t.Fatalf("bad %q", got)
	}
	if (RetryDeliveryOf{}).Target(nil).DeliveredAtLeastOnce {
		t.Fatal("nil target flagged")
	}
}

// assertCommittedNonTransactional reads the destination as a read_committed consumer: retried records are
// visible without any transaction marker (they were produced outside the route's consumer transaction).
func assertCommittedNonTransactional(t *testing.T, opener SeedClientOpener, want int) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "", kgo.ConsumeTopics("orders-public"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.FetchIsolationLevel(kgo.ReadCommitted()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	got := 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for got < want && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			if r.Attrs.IsTransactional() {
				t.Errorf("retried record produced in a transaction: offset %d", r.Offset)
			}
			got++
		})
	}
	if got != want {
		t.Fatalf("read_committed destination got %d", got)
	}
}
