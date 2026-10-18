package connections

import (
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/kafka"
)

// ProblemType names the connection-specific problem types (RFC 7807 `type`).
type ProblemType string

const (
	ProblemConnectionInUse     ProblemType = "https://eventrouter.dev/problems/connection-in-use"
	ProblemTopicExists         ProblemType = "https://eventrouter.dev/problems/topic-exists"
	ProblemTopicCreateDisabled ProblemType = "https://eventrouter.dev/problems/topic-create-disabled"
	ProblemClusterUnavailable  ProblemType = "https://eventrouter.dev/problems/cluster-unavailable"
)

// ErrorDetail holds the user-facing problem details.
type ErrorDetail string

const (
	DetailInUse             ErrorDetail = "This connection is used by routes. Point them to another connection or archive them, then delete again."
	DetailTopicExists       ErrorDetail = "A topic with this name already exists. Pick another name or use the existing topic."
	DetailTopicCreateOff    ErrorDetail = "Topic creation is turned off for this connection. Ask the owner team to allow it in the connection settings."
	DetailSecretUnavailable ErrorDetail = "The stored credentials of this connection could not be read. Replace the secret and retry."
	DetailTestTopicRequired ErrorDetail = "Choose an existing topic for this check."
)

// InUseError blocks deleting a connection that routes still use.
type InUseError struct {
	Routes []RouteRef
}

func (e *InUseError) Error() string               { return string(DetailInUse) }
func (e *InUseError) ProblemKind() problem.Kind   { return problem.KindConflict }
func (e *InUseError) ProblemTypeURI() ProblemType { return ProblemConnectionInUse }

// TopicExistsError reports a create-topic name clash.
type TopicExistsError struct{}

func (e *TopicExistsError) Error() string               { return string(DetailTopicExists) }
func (e *TopicExistsError) ProblemKind() problem.Kind   { return problem.KindConflict }
func (e *TopicExistsError) ProblemPath() string         { return string(PathTopicName) }
func (e *TopicExistsError) ProblemTypeURI() ProblemType { return ProblemTopicExists }

// TopicCreateDisabledError reports that the connection does not allow topic creation.
type TopicCreateDisabledError struct{}

func (e *TopicCreateDisabledError) Error() string               { return string(DetailTopicCreateOff) }
func (e *TopicCreateDisabledError) ProblemKind() problem.Kind   { return problem.KindForbidden }
func (e *TopicCreateDisabledError) ProblemTypeURI() ProblemType { return ProblemTopicCreateDisabled }

// ClusterUnavailableError is a classified cluster failure with its plain-language hint (502).
type ClusterUnavailableError struct {
	Reason  kafka.FailureReason
	Message string
	Hint    string
	Cause   error
}

func (e *ClusterUnavailableError) Error() string               { return e.Message }
func (e *ClusterUnavailableError) Unwrap() error               { return e.Cause }
func (e *ClusterUnavailableError) ProblemKind() problem.Kind   { return problem.KindUnavailable }
func (e *ClusterUnavailableError) ProblemTypeURI() ProblemType { return ProblemClusterUnavailable }

// ErrSecretUnavailable is returned when a stored secret cannot be decrypted (500, no detail leaked).
var ErrSecretUnavailable = problem.NewError(problem.KindInternal, string(DetailSecretUnavailable))

// RequestPath names JSON Pointers of request fields used by domain errors.
type RequestPath string

const (
	PathName             RequestPath = "/name"
	PathDescription      RequestPath = "/description"
	PathEnvTag           RequestPath = "/envTag"
	PathKind             RequestPath = "/kind"
	PathOwnerTeam        RequestPath = "/owner/teamId"
	PathTopicName        RequestPath = "/name"
	PathTopicPartitions  RequestPath = "/partitions"
	PathTopicReplication RequestPath = "/replicationFactor"
	PathTopicRetention   RequestPath = "/retentionMs"
	PathReadTopic        RequestPath = "/readTopic"
	PathWriteTopic       RequestPath = "/writeTopic"
)

// ProblemExtras are the extension members of connection problems.
type ProblemExtras struct {
	Routes []RouteRef          `json:"routes,omitempty"`
	Reason kafka.FailureReason `json:"reason,omitempty"`
	Hint   string              `json:"hint,omitempty"`
}

// ExtendedError is a domain error rendered with its own problem type and extension members.
type ExtendedError interface {
	error
	ProblemTypeURI() ProblemType
	ProblemExtras() ProblemExtras
}

// ProblemExtras implements ExtendedError.
func (e *InUseError) ProblemExtras() ProblemExtras { return ProblemExtras{Routes: e.Routes} }

// ProblemExtras implements ExtendedError.
func (e *TopicExistsError) ProblemExtras() ProblemExtras {
	return ProblemExtras{Reason: kafka.ReasonTopicExists}
}

// ProblemExtras implements ExtendedError.
func (e *TopicCreateDisabledError) ProblemExtras() ProblemExtras { return ProblemExtras{} }

// ProblemExtras implements ExtendedError.
func (e *ClusterUnavailableError) ProblemExtras() ProblemExtras {
	return ProblemExtras{Reason: e.Reason, Hint: e.Hint}
}
