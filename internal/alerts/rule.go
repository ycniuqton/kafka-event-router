package alerts

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// RuleID identifies an alert rule.
type RuleID = uuid.UUID

// ChannelRef is one delivery target of a rule: an email list in Target, or a write-only secret URL
// kept by the secrets Manager under SecretRef (never returned by the API).
type ChannelRef struct {
	Type      ChannelType `json:"type"`
	Target    string      `json:"target,omitempty"`
	SecretRef string      `json:"secretRef,omitempty"`
}

// AlertRule is one per-route rule.
type AlertRule struct {
	ID         RuleID
	RouteID    uuid.UUID
	Name       string
	Kind       AlertRuleKind
	Threshold  float64
	Window     Window
	Channels   []ChannelRef
	Enabled    bool
	CreatedBy  *uuid.UUID
	RowVersion store.RowVersion
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ChannelInput is a channel as the client sends it: Secret is the plaintext webhook URL (write-only);
// omitting it on update keeps the stored secret of the same position and type.
type ChannelInput struct {
	Type   ChannelType `json:"type"`
	Target string      `json:"target,omitempty"`
	Secret *string     `json:"secret,omitempty"`
}

// RuleDraft is the POST /alert-rules body.
type RuleDraft struct {
	RouteID       uuid.UUID      `json:"routeId"`
	Name          string         `json:"name"`
	Kind          AlertRuleKind  `json:"kind"`
	Threshold     float64        `json:"threshold"`
	WindowMinutes int            `json:"windowMinutes"`
	Channels      []ChannelInput `json:"channels"`
	Enabled       *bool          `json:"enabled"`
}

// RulePatch is the PATCH /alert-rules/{id} body (absent fields are kept).
type RulePatch struct {
	Name          *string         `json:"name"`
	Kind          *AlertRuleKind  `json:"kind"`
	Threshold     *float64        `json:"threshold"`
	WindowMinutes *int            `json:"windowMinutes"`
	Channels      *[]ChannelInput `json:"channels"`
	Enabled       *bool           `json:"enabled"`
}

// Apply returns the draft obtained by patching rule (channels resolved separately).
func (p RulePatch) Apply(rule AlertRule) RuleDraft {
	d := RuleDraft{RouteID: rule.RouteID, Name: rule.Name, Kind: rule.Kind, Threshold: rule.Threshold,
		WindowMinutes: int(rule.Window), Enabled: &rule.Enabled}
	if p.Name != nil {
		d.Name = *p.Name
	}
	if p.Kind != nil {
		d.Kind = *p.Kind
	}
	if p.Threshold != nil {
		d.Threshold = *p.Threshold
	}
	if p.WindowMinutes != nil {
		d.WindowMinutes = *p.WindowMinutes
	}
	if p.Enabled != nil {
		d.Enabled = p.Enabled
	}
	return d
}

// ChannelView is a channel as the API returns it: secrets are reported as set, never echoed.
type ChannelView struct {
	Type      ChannelType `json:"type"`
	Target    string      `json:"target,omitempty"`
	SecretSet bool        `json:"secretSet"`
}

// RuleView is the API shape of an alert rule.
type RuleView struct {
	ID            RuleID        `json:"id"`
	RouteID       uuid.UUID     `json:"routeId"`
	Name          string        `json:"name"`
	Kind          AlertRuleKind `json:"kind"`
	Threshold     float64       `json:"threshold"`
	WindowMinutes int           `json:"windowMinutes"`
	Channels      []ChannelView `json:"channels"`
	Enabled       bool          `json:"enabled"`
	Version       int64         `json:"version"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}

// View renders the rule for the API.
func (r AlertRule) View() RuleView {
	ch := make([]ChannelView, 0, len(r.Channels))
	for _, c := range r.Channels {
		ch = append(ch, ChannelView{Type: c.Type, Target: c.Target, SecretSet: c.SecretRef != ""})
	}
	return RuleView{ID: r.ID, RouteID: r.RouteID, Name: r.Name, Kind: r.Kind, Threshold: r.Threshold,
		WindowMinutes: int(r.Window), Channels: ch, Enabled: r.Enabled, Version: int64(r.RowVersion),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
