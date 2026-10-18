package dashboard

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/routes"
)

type firing []alerts.FeedItem

func (f firing) FiringItems(context.Context) ([]alerts.FeedItem, error) { return f, nil }

func TestFiringAlertDetector(t *testing.T) {
	live, gone := uuid.New(), uuid.New()
	v := 25000.0
	items := firing{
		{RuleID: uuid.New(), RuleName: "Lag high", RouteID: live, RouteName: "orders", Kind: alerts.KindLagAbove, Threshold: 20000, Window: 10, Value: &v},
		{RuleID: uuid.New(), RouteID: gone, Kind: alerts.KindThroughputZero, Window: 5},
	}
	snap := Snapshot{Routes: []RouteRow{{ID: routes.RouteID(live.String()), Name: "orders", Status: routes.StatusRunning}}}
	out, err := FiringAlertDetector{Alerts: items}.Detect(context.Background(), snap)
	if err != nil || len(out) != 1 {
		t.Fatalf("archived/missing routes are skipped: %v %d", err, len(out))
	}
	it := out[0]
	if it.Kind != metrics.AttentionFiringAlert || it.Tone != metrics.ToneErr || it.Title != "orders · Lag high" ||
		!strings.Contains(it.Detail, "Consumer lag stayed above 20000 for 10 min · now 25000") || it.Action.Intent.Tab != TabOverview {
		t.Fatalf("row: %+v", it)
	}
}
