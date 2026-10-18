package routes

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/summary"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
)

// SourceView is the source column of the list.
type SourceView struct {
	ConnectionID   string   `json:"connectionId"`
	ConnectionName string   `json:"connectionName"`
	Topics         []string `json:"topics"`
	TopicPattern   *string  `json:"topicPattern"`
}

// DestinationView is one destination column entry (one per branch).
type DestinationView struct {
	BranchID       string `json:"branchId"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Topic          string `json:"topic"`
}

// BranchView summarises one branch: its filter headline, transform indicator and destination.
type BranchView struct {
	ID            string                  `json:"id"`
	FilterSummary *summary.FilterHeadline `json:"filterSummary"`
	Transform     TransformIndicator      `json:"transform"`
	Destination   DestinationView         `json:"destination"`
}

// RouteSummary is one row of GET /routes.
type RouteSummary struct {
	ID                 RouteID                 `json:"id"`
	Name               string                  `json:"name"`
	TeamID             TeamID                  `json:"teamId"`
	TeamName           string                  `json:"teamName"`
	Tags               []string                `json:"tags"`
	Status             Status                  `json:"status"`
	DesiredState       DesiredState            `json:"desiredState"`
	ObservedState      *ObservedState          `json:"observedState"`
	ActiveVersion      *ActiveVersionRef       `json:"activeVersion"`
	HasUndeployedDraft bool                    `json:"hasUndeployedDraft"`
	Source             *SourceView             `json:"source"`
	FilterSummary      *summary.FilterHeadline `json:"filterSummary"`
	Transform          TransformIndicator      `json:"transform"`
	Destinations       []DestinationView       `json:"destinations"`
	Branches           []BranchView            `json:"branches"`
	Metrics            *MetricsSummary         `json:"metrics"`
	UpdatedAt          time.Time               `json:"updatedAt"`
	UpdatedBy          *UserRef                `json:"updatedBy"`
}

// RouteDetail is GET /routes/{id}: the summary plus settings, draft, active config and last deployment.
type RouteDetail struct {
	RouteSummary
	Description    string          `json:"description"`
	Replicas       int             `json:"replicas"`
	Revision       Revision        `json:"revision"`
	Draft          json.RawMessage `json:"draft"`
	Active         json.RawMessage `json:"active"`
	LastDeployment *Deployment     `json:"lastDeployment"`
	Issues         config.Issues   `json:"issues,omitempty"`
}

// SummaryBuilder turns repository rows into list/detail views.
type SummaryBuilder struct {
	Connections validation.ConnectionLookup
	Metrics     RouteMetricsReader
	Log         *slog.Logger
}

// Summaries builds views for rows, resolving connection names and metrics in one batch each.
func (b SummaryBuilder) Summaries(ctx context.Context, rows []SummaryRow) []RouteSummary {
	configs := make([]*config.RouteConfig, len(rows))
	ids := make([]RouteID, len(rows))
	for i, row := range rows {
		configs[i] = ConfigOf(b.displayed(row))
		ids[i] = row.Route.ID
	}
	names := b.connectionNames(ctx, configs)
	metrics := b.metrics(ctx, ids)
	out := make([]RouteSummary, len(rows))
	for i, row := range rows {
		out[i] = b.one(row, configs[i], names, metrics)
	}
	return out
}

// Detail builds the detail view of one row.
func (b SummaryBuilder) Detail(ctx context.Context, row SummaryRow, last *Deployment) RouteDetail {
	d := RouteDetail{RouteSummary: b.Summaries(ctx, []SummaryRow{row})[0]}
	d.Description, d.Replicas, d.Revision = row.Route.Description, row.Route.Replicas, row.Route.Revision
	d.Draft, d.LastDeployment = row.Route.Draft, last
	if row.Active != nil {
		d.Active = row.Active.Config
	}
	return d
}

// displayed is the active config, or the draft for never-deployed routes.
func (SummaryBuilder) displayed(row SummaryRow) json.RawMessage {
	if row.Active != nil {
		return row.Active.Config
	}
	return row.Route.Draft
}

func (b SummaryBuilder) one(row SummaryRow, cfg *config.RouteConfig, names map[string]validation.ConnectionInfo, metrics map[RouteID]MetricsSummary) RouteSummary {
	r := row.Route
	s := RouteSummary{
		ID: r.ID, Name: r.Name, TeamID: r.TeamID, TeamName: row.TeamName, Tags: NonNilTags(r.Tags),
		Status:       RouteStatusResolver{}.Resolve(r.DesiredState, r.HasActiveVersion(), row.ObservedState),
		DesiredState: r.DesiredState, ActiveVersion: row.Active, UpdatedAt: r.UpdatedAt, UpdatedBy: row.UpdatedBy,
		HasUndeployedDraft: b.undeployed(row), Transform: TransformNone, Destinations: []DestinationView{}, Branches: []BranchView{},
	}
	if row.ObservedState != ObservedUnknown {
		obs := row.ObservedState
		s.ObservedState = &obs
	}
	if m, ok := metrics[r.ID]; ok {
		s.Metrics = &m
	}
	if cfg != nil {
		ConfigProjection{Names: names}.Apply(&s, cfg)
	}
	return s
}

// undeployed reports a draft that differs from the live version.
func (SummaryBuilder) undeployed(row SummaryRow) bool {
	if row.Route.Draft == nil {
		return false
	}
	if row.Active == nil {
		return true
	}
	return !ConfigFingerprint{}.Same(row.Route.Draft, row.Active.Config)
}

func (b SummaryBuilder) connectionNames(ctx context.Context, cfgs []*config.RouteConfig) map[string]validation.ConnectionInfo {
	ids := []string{}
	for _, c := range cfgs {
		if c != nil {
			ids = append(ids, validation.ConnectionRefs{}.IDs(validation.ConnectionRefs{}.Of(c))...)
		}
	}
	if len(ids) == 0 || b.Connections == nil {
		return map[string]validation.ConnectionInfo{}
	}
	found, err := b.Connections.Find(ctx, ids)
	if err != nil {
		b.Log.Warn("routes: connection names unavailable", string(logging.AttrError), err)
		return map[string]validation.ConnectionInfo{}
	}
	return found
}

func (b SummaryBuilder) metrics(ctx context.Context, ids []RouteID) map[RouteID]MetricsSummary {
	if len(ids) == 0 || b.Metrics == nil {
		return map[RouteID]MetricsSummary{}
	}
	m, err := b.Metrics.Summaries(ctx, ids)
	if err != nil {
		b.Log.Warn("routes: metrics unavailable", string(logging.AttrError), err)
		return map[RouteID]MetricsSummary{}
	}
	return m
}

// ConfigProjection copies the config-derived columns into a summary.
type ConfigProjection struct {
	Names map[string]validation.ConnectionInfo
}

// Apply fills source, filter headline, transform indicator and destinations.
func (p ConfigProjection) Apply(s *RouteSummary, cfg *config.RouteConfig) {
	s.Source = &SourceView{ConnectionID: cfg.Source.ConnectionID, ConnectionName: p.Names[cfg.Source.ConnectionID].Name,
		Topics: NonNilTags(cfg.Source.Topics), TopicPattern: cfg.Source.TopicPattern}
	s.Transform = TransformIndicator(summary.TransformClassifier{}.Classify(cfg))
	for i, br := range cfg.Branches {
		view := p.branch(br)
		if i == 0 {
			s.FilterSummary = view.FilterSummary
		}
		s.Destinations = append(s.Destinations, view.Destination)
		s.Branches = append(s.Branches, view)
	}
}

// branch summarises one branch (its transform classified on its own).
func (p ConfigProjection) branch(br config.Branch) BranchView {
	one := &config.RouteConfig{Branches: []config.Branch{br}}
	return BranchView{ID: br.ID, FilterSummary: summary.FilterSummarizer{}.Headline(br.Filter),
		Transform: TransformIndicator(summary.TransformClassifier{}.Classify(one)),
		Destination: DestinationView{BranchID: br.ID, ConnectionID: br.Destination.ConnectionID,
			ConnectionName: p.Names[br.Destination.ConnectionID].Name, Topic: br.Destination.Topic}}
}

// NonNilTags returns an empty slice instead of nil so JSON shows [].
func NonNilTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
