package dlq

import (
	"encoding/json"

	"github.com/eventrouter/router/pkg/config"
)

// RetryDowngrades lists route delivery modes whose guarantee a retry cannot keep: retries produce outside
// the route's consumer transaction, so an exactly_once route's retried events arrive at-least-once (FR-21).
var RetryDowngrades = map[config.DeliveryMode]bool{config.DeliveryModeExactlyOnce: true}

// DeliveryModeOf reads the delivery mode of stored versions (absent or unreadable = at_least_once).
type DeliveryModeOf struct{}

// Config returns the delivery mode of one raw version config.
func (DeliveryModeOf) Config(raw json.RawMessage) config.DeliveryMode {
	doc, err := config.Parse(raw)
	if err != nil {
		return config.DeliveryModeAtLeastOnce
	}
	cfg, err := doc.Config()
	if err != nil || cfg.Delivery == nil || cfg.Delivery.Mode == "" {
		return config.DeliveryModeAtLeastOnce
	}
	return cfg.Delivery.Mode
}

// Active returns the mode of the route's active version; an undeployed route has none.
func (d DeliveryModeOf) Active(route RouteInfo, versions []VersionConfig) config.DeliveryMode {
	for _, v := range versions {
		if v.ID == route.ActiveVersionID && route.HasActiveVersion() {
			return d.Config(v.Config)
		}
	}
	return ""
}

// RetryDelivery is what a retry report says about delivery: the route mode and the downgrade flag.
type RetryDelivery struct {
	Mode                 config.DeliveryMode
	DeliveredAtLeastOnce bool
}

// RetryDeliveryOf derives the retry delivery notice of a retry target.
type RetryDeliveryOf struct{}

// Target returns the notice of the version a retry runs through.
func (RetryDeliveryOf) Target(t *RetryTarget) RetryDelivery {
	if t == nil {
		return RetryDelivery{}
	}
	mode := DeliveryModeOf{}.Config(t.Version.Config)
	return RetryDelivery{Mode: mode, DeliveredAtLeastOnce: RetryDowngrades[mode]}
}
