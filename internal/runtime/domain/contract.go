package domain

import (
	"net/url"
	"strings"
	"time"
)

// VersionRef identifies one immutable route version.
type VersionRef struct {
	RouteID    string `json:"routeId"`
	VersionID  string `json:"versionId"`
	VersionNo  int    `json:"versionNo"`
	ConfigHash string `json:"configHash"`
}

// Assignment is one lease of a route replica on a worker, with the version it must run.
type Assignment struct {
	RouteID        string    `json:"routeId"`
	RouteName      string    `json:"routeName"`
	TeamID         string    `json:"teamId"`
	VersionID      string    `json:"versionId"`
	VersionNo      int       `json:"versionNo"`
	ConfigHash     string    `json:"configHash"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt"`
}

// Ref returns the version the assignment targets.
func (a Assignment) Ref() VersionRef {
	return VersionRef{RouteID: a.RouteID, VersionID: a.VersionID, VersionNo: a.VersionNo, ConfigHash: a.ConfigHash}
}

// AssignmentSet is the explicit answer of the control plane for one worker.
type AssignmentSet struct {
	ETag        string       `json:"etag"`
	Assignments []Assignment `json:"assignments"`
}

// ByRoute indexes the set by route id.
func (s AssignmentSet) ByRoute() map[string]Assignment {
	out := make(map[string]Assignment, len(s.Assignments))
	for _, a := range s.Assignments {
		out[a.RouteID] = a
	}
	return out
}

// ReplicaStatus is what one runner reports about itself.
type ReplicaStatus struct {
	RouteID       string        `json:"routeId"`
	VersionID     string        `json:"versionId"`
	VersionNo     int           `json:"versionNo"`
	State         RunnerState   `json:"state"`
	ErrorCategory ErrorCategory `json:"errorCategory,omitempty"`
	ErrorMessage  string        `json:"errorMessage,omitempty"`
	Lag           int64         `json:"lag"`
	Partitions    int           `json:"partitions"`
	CPUMs         int64         `json:"cpuMs"`
	MemBytes      int64         `json:"memBytes"`
}

// StatusReport is the body of POST /internal/v1/workers/{id}/status.
type StatusReport struct {
	Replicas []ReplicaStatus `json:"replicas"`
}

// Heartbeat is the body of POST /internal/v1/workers/{id}/heartbeat.
type Heartbeat struct {
	Host            string   `json:"host"`
	Zone            string   `json:"zone"`
	Version         string   `json:"version"`
	Capacity        int      `json:"capacity"`
	CapacityUsed    float64  `json:"capacityUsed"`
	RefusedRouteIDs []string `json:"refusedRouteIds"`
	Draining        bool     `json:"draining"`
}

// HeartbeatReply tells the worker its lease timings and the current assignments ETag.
type HeartbeatReply struct {
	LeaseTTLMs      int64  `json:"leaseTtlMs"`
	HeartbeatMs     int64  `json:"heartbeatMs"`
	AssignmentsETag string `json:"assignmentsEtag"`
}

// HaltReport is the body of POST /internal/v1/routes/{id}/halt.
type HaltReport struct {
	RouteID       string        `json:"-"`
	WorkerID      string        `json:"workerId"`
	VersionID     string        `json:"versionId"`
	Reason        HaltReason    `json:"reason"`
	ErrorCategory ErrorCategory `json:"errorCategory"`
	Message       string        `json:"message"`
}

// ChangeHint is one event of the GET /internal/v1/changes stream.
type ChangeHint struct {
	RouteID string     `json:"routeId"`
	Kind    ChangeKind `json:"kind"`
}

// WorkerIdentity describes the worker process to the control plane.
type WorkerIdentity struct {
	ID       string
	Host     string
	Zone     string
	Version  string
	Capacity int
}

// InternalPath is a path of the internal worker API (/internal/v1, workload identity).
type InternalPath string

const (
	PathHeartbeat     InternalPath = "/internal/v1/workers/{id}/heartbeat"
	PathAssignments   InternalPath = "/internal/v1/workers/{id}/assignments"
	PathStatus        InternalPath = "/internal/v1/workers/{id}/status"
	PathVersionConfig InternalPath = "/internal/v1/route-versions/{id}/config"
	PathHalt          InternalPath = "/internal/v1/routes/{id}/halt"
	PathChanges       InternalPath = "/internal/v1/changes"
	PathConnectionKey InternalPath = "/internal/v1/connections/{id}/spec"
)

// PathParamName names the single path parameter of internal paths.
type PathParamName string

const PathParam PathParamName = "id"

// With fills the {id} parameter (escaped).
func (p InternalPath) With(id string) string {
	return strings.ReplaceAll(string(p), "{"+string(PathParam)+"}", url.PathEscape(id))
}

// HTTPHeader names the conditional-request headers of the assignments endpoint.
type HTTPHeader string

const (
	HeaderETag        HTTPHeader = "ETag"
	HeaderIfNoneMatch HTTPHeader = "If-None-Match"
)
