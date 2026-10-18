package dlq

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
)

// EnvPreviewIT enables the PreviewExecutor test against a running preview service (local dev stack).
const EnvPreviewIT = "ROUTER_TEST_PREVIEW"

// TestPreviewExecutorAgainstPreviewService is DL-114 end to end: the retry runs in cmd/preview, the
// transformed output comes back and is produced here; filtered and failing events are reported.
func TestPreviewExecutorAgainstPreviewService(t *testing.T) {
	if os.Getenv(EnvPreviewIT) == "" {
		t.Skipf("%s not set (needs cmd/preview, e.g. source deploy/dev/env.sh)", EnvPreviewIT)
	}
	settings, err := preview.NewSettingsLoader().Load(config.Environment(os.Getenv("ROUTER_ENV")))
	if err != nil {
		t.Fatal(err)
	}
	client := gateway.NewPreviewClient(settings.URL, nil, preview.NewDirectiveCodec(settings.DirectiveSecret, time.Now))
	recs := []record.Record{failure(0, record.ErrorStep), failure(1, record.ErrorStep), failure(2, record.ErrorStep)}
	recs[0].Value, recs[1].Value, recs[2].Value = []byte(`{"amount":200,"cust_id":7}`), []byte(`{"amount":5}`), []byte(`{bad`)
	producer := &recordingProducer{}
	target := RetryTarget{Route: RouteInfo{ID: "r1", TeamID: ""}, Version: VersionConfig{ID: "v1", No: 1, Config: json.RawMessage(e2eRouteConfig)}}
	results := PreviewExecutor{Preview: client, Producer: producer}.Execute(context.Background(), target, recs)
	got := []Outcome{results[0].Outcome, results[1].Outcome, results[2].Outcome}
	if got[0] != OutcomeSent || got[1] != OutcomeFiltered || got[2] != OutcomeFailed || results[2].ErrorType != FailDecode {
		t.Fatalf("outcomes %v %+v", got, results)
	}
	if len(producer.produced) != 1 || !strings.Contains(string(producer.produced[0].Value), `"customerId":7`) {
		t.Fatalf("produced %+v", producer.produced)
	}
}
