// Package alerts holds per-route alert rules, their evaluation state and alert history: rule kinds,
// notification channels, validation and the services behind /alert-rules and /alerts.
package alerts

import "time"

// AlertRuleKind is what an alert rule watches (alert_rule_kind enum).
type AlertRuleKind string

const (
	KindLagAbove        AlertRuleKind = "lag_above"
	KindErrorRateAbove  AlertRuleKind = "error_rate_above"
	KindThroughputZero  AlertRuleKind = "throughput_zero"
	KindMatchRateChange AlertRuleKind = "match_rate_change"
	KindRouterCausedDLQ AlertRuleKind = "router_caused_dead_letters"
)

// Kinds lists every rule kind in form order.
var Kinds = []AlertRuleKind{KindLagAbove, KindErrorRateAbove, KindThroughputZero, KindMatchRateChange, KindRouterCausedDLQ}

// Valid reports whether k is a known kind.
func (k AlertRuleKind) Valid() bool {
	_, ok := KindLimits[k]
	return ok
}

// ChannelType is where a notification is delivered.
type ChannelType string

const (
	ChannelEmail   ChannelType = "email"
	ChannelSlack   ChannelType = "slack"
	ChannelTeams   ChannelType = "teams"
	ChannelWebhook ChannelType = "webhook"
)

// ChannelTypes lists every channel type.
var ChannelTypes = []ChannelType{ChannelEmail, ChannelSlack, ChannelTeams, ChannelWebhook}

// UsesSecret reports whether the channel target is a secret URL (stored via the secrets Manager).
func (c ChannelType) UsesSecret() bool {
	return c == ChannelSlack || c == ChannelTeams || c == ChannelWebhook
}

// Valid reports whether c is a known channel type.
func (c ChannelType) Valid() bool {
	for _, t := range ChannelTypes {
		if t == c {
			return true
		}
	}
	return false
}

// AlertState is the lifecycle state of one alert event (alert_state enum).
type AlertState string

const (
	StateFiring   AlertState = "firing"
	StateResolved AlertState = "resolved"
)

// Bound is an inclusive numeric range.
type Bound struct {
	Min float64
	Max float64
}

// Contains reports whether v lies in the bound.
func (b Bound) Contains(v float64) bool {
	return v >= b.Min && v <= b.Max
}

// KindLimits are the accepted thresholds per kind (lag in messages, rates and points in percent).
var KindLimits = map[AlertRuleKind]Bound{
	KindLagAbove:        {Min: 1, Max: 1e12},
	KindErrorRateAbove:  {Min: 0.01, Max: 100},
	KindThroughputZero:  {Min: 0, Max: 0},
	KindMatchRateChange: {Min: 1, Max: 100},
	KindRouterCausedDLQ: {Min: 0, Max: 1e6},
}

// WindowLimits bound the evaluation window in minutes.
var WindowLimits = Bound{Min: 1, Max: 1440}

// RuleLimits groups the remaining rule constraints.
type RuleLimits int

const (
	MaxNameLength      RuleLimits = 120
	MaxChannels        RuleLimits = 10
	MaxEmailRecipients RuleLimits = 20
)

// Window converts minutes to a duration.
type Window int

// Duration returns the window length.
func (w Window) Duration() time.Duration {
	return time.Duration(w) * time.Minute
}
