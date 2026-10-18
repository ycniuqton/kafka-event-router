package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Paths are the API paths the runner uses.
var Paths = struct {
	Connections, Routes string
	Connection, Route   func(id string) string
	RouteAction         func(id, action string) string
}{
	Connections: "/api/v1/connections",
	Routes:      "/api/v1/routes",
	Connection:  func(id string) string { return "/api/v1/connections/" + id },
	Route:       func(id string) string { return "/api/v1/routes/" + id },
	RouteAction: func(id, action string) string { return "/api/v1/routes/" + id + "/" + action },
}

// RouteState values of observedState / deploy status the runner waits for.
type RouteState string

const (
	StateRunning         RouteState = "running"
	StatePendingApproval RouteState = "pending_approval"
)

// Convergence bounds waiting for a deployed route to run.
var Convergence = struct {
	Poll, Timeout time.Duration
	ListLimit     int
}{Poll: 500 * time.Millisecond, Timeout: 3 * time.Minute, ListLimit: 200}

// Platform is the route/connection API of one signed-in session.
type Platform struct {
	c      *Client
	teamID string
}

// NewPlatform binds a session and resolves the caller's first team.
func NewPlatform(ctx context.Context, c *Client) (*Platform, error) {
	res, err := c.Get(ctx, AuthPaths.Me)
	if err != nil {
		return nil, err
	}
	var me struct {
		Teams []struct {
			ID string `json:"id"`
		} `json:"teams"`
	}
	if err := res.Decode(&me); err != nil || len(me.Teams) == 0 {
		return nil, fmt.Errorf("whoami: no team (%v)", err)
	}
	return &Platform{c: c, teamID: me.Teams[0].ID}, nil
}

// CreateConnection creates a plaintext Kafka connection and returns its id.
func (p *Platform) CreateConnection(ctx context.Context, name, bootstrap, envTag string) (string, error) {
	body := map[string]any{
		"name": name, "envTag": envTag, "bootstrapServers": strings.Split(bootstrap, ","), "security": map[string]string{"mode": "plaintext"},
		"allowTopicCreate": true, "owner": map[string]string{"teamId": p.teamID},
	}
	return p.create(ctx, Paths.Connections, body, "create connection "+name)
}

// CreateRoute creates a route from a draft document and returns its id.
func (p *Platform) CreateRoute(ctx context.Context, name string, draft map[string]any) (string, error) {
	body := map[string]any{"name": name, "teamId": p.teamID, "draft": draft, "tags": []string{"stress"}}
	return p.create(ctx, Paths.Routes, body, "create route "+name)
}

// SetReplicas patches the replica count of a route.
func (p *Platform) SetReplicas(ctx context.Context, id string, n int) error {
	res, err := p.c.WithETag(ctx, http.MethodPatch, Paths.Route(id), map[string]int{"replicas": n}, Paths.Route(id))
	if err != nil {
		return err
	}
	return res.Err("set replicas")
}

// TopicVisibility bounds the wait for freshly created topics to become visible to the API's validation.
var TopicVisibility = struct {
	Code    string
	Retry   time.Duration
	Timeout time.Duration
}{Code: `"topic_missing"`, Retry: time.Second, Timeout: time.Minute}

// Deploy deploys the draft; a production approval request is reported as an error (stress connections are dev).
// A topic_missing rejection is retried: topics created just before can lag in the API's metadata view.
func (p *Platform) Deploy(ctx context.Context, id string) error {
	deadline := time.Now().Add(TopicVisibility.Timeout)
	res, err := p.deployOnce(ctx, id)
	for err == nil && res.Status == http.StatusUnprocessableEntity && bytes.Contains(res.Body, []byte(TopicVisibility.Code)) && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(TopicVisibility.Retry):
		}
		res, err = p.deployOnce(ctx, id)
	}
	if err != nil {
		return err
	}
	var dep struct {
		Status RouteState `json:"status"`
	}
	_ = res.Decode(&dep)
	if dep.Status == StatePendingApproval {
		return fmt.Errorf("deploy of %s needs approval: use a non-production envTag for stress connections", id)
	}
	return res.Err("deploy")
}

func (p *Platform) deployOnce(ctx context.Context, id string) (Response, error) {
	return p.c.WithETag(ctx, http.MethodPost, Paths.RouteAction(id, "deploy"), map[string]string{"changeNote": "stress"}, Paths.Route(id))
}

// WaitRunning polls observedState until running and returns how long it took.
func (p *Platform) WaitRunning(ctx context.Context, id string) (time.Duration, error) {
	start := time.Now()
	deadline := start.Add(Convergence.Timeout)
	for time.Now().Before(deadline) {
		res, err := p.c.Get(ctx, Paths.Route(id))
		var r struct {
			ObservedState RouteState `json:"observedState"`
		}
		if err == nil && res.Decode(&r) == nil && r.ObservedState == StateRunning {
			return time.Since(start), nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(Convergence.Poll):
		}
	}
	return 0, fmt.Errorf("route %s not running after %s", id, Convergence.Timeout)
}

// Archive pauses then archives a route (routes have no hard delete).
func (p *Platform) Archive(ctx context.Context, id string) error {
	_, _ = p.c.WithETag(ctx, http.MethodPost, Paths.RouteAction(id, "pause"), map[string]any{}, Paths.Route(id))
	res, err := p.c.WithETag(ctx, http.MethodDelete, Paths.Route(id), nil, Paths.Route(id))
	if err != nil {
		return err
	}
	return res.Err("archive route " + id)
}

// DeleteConnection deletes a connection.
func (p *Platform) DeleteConnection(ctx context.Context, id string) error {
	res, err := p.c.Do(ctx, http.MethodDelete, Paths.Connection(id), nil, "")
	if err != nil {
		return err
	}
	return res.Err("delete connection " + id)
}

// Named is an id/name pair from a list endpoint.
type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListPrefixed returns routes (any non-archived status) or connections whose name starts with prefix.
func (p *Platform) ListPrefixed(ctx context.Context, collection, prefix string) ([]Named, error) {
	q := url.Values{"q": {prefix}, "limit": {fmt.Sprint(Convergence.ListLimit)}}
	if collection == Paths.Routes {
		q.Set("status", "draft,running,paused,error")
	}
	res, err := p.c.Get(ctx, collection+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []Named `json:"items"`
	}
	if err := res.Decode(&list); err != nil {
		return nil, res.Err("list " + collection)
	}
	var out []Named
	for _, it := range list.Items {
		if strings.HasPrefix(it.Name, prefix) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (p *Platform) create(ctx context.Context, path string, body any, what string) (string, error) {
	res, err := p.c.Do(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return "", err
	}
	if err := res.Err(what); err != nil {
		return "", err
	}
	var created Named
	if err := res.Decode(&created); err != nil || created.ID == "" {
		return "", fmt.Errorf("%s: no id in answer", what)
	}
	return created.ID, nil
}
