package connhttp

import (
	"net/http"
	"strconv"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
)

// TopicListResponse is the topic browser body.
type TopicListResponse struct {
	Items []connections.TopicView `json:"items"`
}

// TopicsHandler serves GET /api/v1/connections/{id}/topics?q&includeInternal.
type TopicsHandler struct{ d Deps }

// Route implements api.Handler.
func (h *TopicsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTopics), Permission: PermRead, Object: h.d.Object}
}

// Serve implements api.Handler.
func (h *TopicsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	v := r.URL.Query()
	internal, _ := strconv.ParseBool(v.Get(string(QueryIncludeInternal)))
	topics, err := h.d.Topics.List(r.Context(), id, connections.TopicQuery{Text: v.Get(string(QueryText)), IncludeInternal: internal})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, TopicListResponse{Items: topics})
}

// CreateTopicHandler serves POST /api/v1/connections/{id}/topics.
type CreateTopicHandler struct{ d Deps }

// Route implements api.Handler.
func (h *CreateTopicHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTopics), Permission: PermTopicCreate, Object: h.d.Object, Mutating: true, AuditAction: connections.ActionTopicCreated}
}

// Serve implements api.Handler.
func (h *CreateTopicHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	in, err := api.Decoder[TopicInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	req := connections.TopicRequest{Name: in.Name, Partitions: in.Partitions, ReplicationFactor: in.ReplicationFactor, RetentionMs: in.RetentionMs, Configs: in.Configs}
	created, err := h.d.Creator.Create(r.Context(), id, req)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusCreated, created)
}

// SpecHandler serves GET /internal/v1/connections/{id}/spec to workloads (includes decrypted secrets).
type SpecHandler struct{ d Deps }

// Route implements api.Handler.
func (h *SpecHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathSpec), Auth: api.AuthRequired}
}

// Serve implements api.Handler; every read is audited as a secret read by the calling workload.
func (h *SpecHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	c, spec, err := h.d.Resolver.Resolve(r.Context(), id)
	if err != nil {
		return err
	}
	team := c.TeamID
	entry := api.AuditContext(r).Apply(audit.Entry{Action: audit.ActionSecretRead, Object: audit.ObjectConnection, ObjectID: c.ID.String(), TeamID: &team})
	if err := h.d.Recorder.Record(r.Context(), entry); err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, spec)
}
