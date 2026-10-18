package dashboard

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/metrics"
)

// FiringAlertText holds the wording of firing-alert rows.
type FiringAlertText string

const (
	TitleFiringAlert  FiringAlertText = "%s · %s"
	DetailFiringAlert FiringAlertText = "%s · now %s"
	ValueUnknown      FiringAlertText = "no data"
)

// TabOverview is the route-detail tab firing-alert rows open.
const TabOverview IntentTarget = "overview"

// FiringAlertReader lists firing alerts (alerts' EventRepository).
type FiringAlertReader interface {
	FiringItems(ctx context.Context) ([]alerts.FeedItem, error)
}

// FiringAlertDetector lists firing alert rules of live routes (P2).
type FiringAlertDetector struct {
	Alerts FiringAlertReader
}

func (FiringAlertDetector) Kind() metrics.AttentionKind { return metrics.AttentionFiringAlert }

// Detect implements AttentionDetector.
func (d FiringAlertDetector) Detect(ctx context.Context, snap Snapshot) ([]AttentionItem, error) {
	items, err := d.Alerts.FiringItems(ctx)
	if err != nil {
		return nil, err
	}
	names := snap.Names()
	out := []AttentionItem{}
	for _, it := range items {
		if _, live := names[it.RouteID.String()]; !live {
			continue
		}
		out = append(out, AttentionItem{ID: ItemID{}.Of(d.Kind(), it.RuleID.String()), Kind: d.Kind(), Tone: metrics.ToneErr,
			Title: fmt.Sprintf(string(TitleFiringAlert), it.RouteName, it.RuleName), Detail: d.detail(it),
			Action: Action{Label: LabelOpen, Intent: ActionIntent{Type: IntentRouteDetail, RouteID: it.RouteID.String(), Tab: TabOverview}}})
	}
	return out, nil
}

func (FiringAlertDetector) detail(it alerts.FeedItem) string {
	cond := notify.MessageBuilder{}.Condition(notify.Notification{Kind: it.Kind, Threshold: it.Threshold, Window: it.Window})
	value := string(ValueUnknown)
	if it.Value != nil {
		value = strconv.FormatFloat(*it.Value, 'f', -1, 64)
	}
	return fmt.Sprintf(string(DetailFiringAlert), cond, value)
}
