// Package cpclient is the worker's HTTP ControlPlane client over the API's internal listener
// (workload identity via internal/platform/controlplane): heartbeat, assignments, configs, status, halts, changes.
package cpclient

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/kafka"
)

// Client implements reconcile.ControlPlane for one worker.
type Client struct {
	cp       *controlplane.Client
	workerID string
}

// New builds the client.
func New(cp *controlplane.Client, workerID string) *Client {
	return &Client{cp: cp, workerID: workerID}
}

// Heartbeat reports liveness and capacity.
func (c *Client) Heartbeat(ctx context.Context, hb domain.Heartbeat) (domain.HeartbeatReply, error) {
	var reply domain.HeartbeatReply
	_, err := c.cp.Call(ctx, controlplane.Request{Method: http.MethodPost, Path: domain.PathHeartbeat.With(c.workerID), Body: hb}, &reply)
	return reply, err
}

// Assignments returns the explicit assignment set; notModified means etag is still current.
func (c *Client) Assignments(ctx context.Context, etag string) (domain.AssignmentSet, bool, error) {
	req := controlplane.Request{Method: http.MethodGet, Path: domain.PathAssignments.With(c.workerID)}
	if etag != "" {
		req.Headers = map[controlplane.Header]string{controlplane.Header(domain.HeaderIfNoneMatch): etag}
	}
	var set domain.AssignmentSet
	resp, err := c.cp.Call(ctx, req, &set)
	if err != nil {
		return domain.AssignmentSet{}, false, err
	}
	return set, resp.Status == http.StatusNotModified, nil
}

// VersionConfig returns the immutable route config document of a version.
func (c *Client) VersionConfig(ctx context.Context, versionID string) ([]byte, error) {
	var doc json.RawMessage
	_, err := c.cp.Call(ctx, controlplane.Request{Method: http.MethodGet, Path: domain.PathVersionConfig.With(versionID)}, &doc)
	return doc, err
}

// ReportStatus sends every replica status of this worker.
func (c *Client) ReportStatus(ctx context.Context, replicas []domain.ReplicaStatus) error {
	_, err := c.cp.Call(ctx, controlplane.Request{Method: http.MethodPost, Path: domain.PathStatus.With(c.workerID), Body: domain.StatusReport{Replicas: replicas}}, nil)
	return err
}

// ReportHalt tells the control plane a route halted on this worker.
func (c *Client) ReportHalt(ctx context.Context, h domain.HaltReport) error {
	h.WorkerID = c.workerID
	_, err := c.cp.Call(ctx, controlplane.Request{Method: http.MethodPost, Path: domain.PathHalt.With(h.RouteID), Body: h}, nil)
	return err
}

// ConnectionSpec fetches a connection with its secrets (kept in memory only).
func (c *Client) ConnectionSpec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error) {
	var spec kafka.ConnectionSpec
	_, err := c.cp.Call(ctx, controlplane.Request{Method: http.MethodGet, Path: domain.PathConnectionKey.With(connectionID)}, &spec)
	return spec, err
}

// Changes opens the change-hint stream; the channel closes when the stream ends.
func (c *Client) Changes(ctx context.Context) (<-chan domain.ChangeHint, error) {
	body, err := c.cp.Stream(ctx, string(domain.PathChanges))
	if err != nil {
		return nil, err
	}
	out := make(chan domain.ChangeHint)
	go func() {
		defer close(out)
		defer body.Close()
		EventStream{}.Read(ctx, body, out)
	}()
	return out, nil
}
