package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// Channel names a Postgres LISTEN/NOTIFY channel carrying control-plane change events.
type Channel string

const (
	ChannelRouteChanged      Channel = "route_changed"
	ChannelConnectionChanged Channel = "connection_changed"
	ChannelDeploymentChanged Channel = "deployment_changed"
	ChannelAlertRuleChanged  Channel = "alert_rule_changed"
)

// ChangePayload is the JSON body of every change notification.
type ChangePayload struct {
	ID         string     `json:"id"`
	RowVersion RowVersion `json:"row_version"`
}

// Notifier publishes change events with pg_notify; inside a tx they are delivered on commit.
type Notifier struct {
	db *DB
}

// NewNotifier returns a Notifier over db.
func NewNotifier(db *DB) *Notifier {
	return &Notifier{db: db}
}

// Notify sends payload (JSON-encoded) on the channel using the ambient transaction if any.
func (n *Notifier) Notify(ctx context.Context, ch Channel, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("store: notify %s: %w", ch, err)
	}
	if _, err := n.db.Q(ctx).Exec(ctx, "SELECT pg_notify($1, $2)", string(ch), string(body)); err != nil {
		return fmt.Errorf("store: notify %s: %w", ch, err)
	}
	return nil
}
