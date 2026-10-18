package preview

import (
	"net/http"
	"strconv"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Problem kinds owned by live preview (registered on the service's problem catalog).
const (
	KindSessionExpired    problem.Kind = "preview_session_expired"
	KindInvalidConfig     problem.Kind = "preview_invalid_config"
	KindBusy              problem.Kind = "preview_busy"
	KindSourceUnavailable problem.Kind = "preview_source_unavailable"
	KindTopicNotFound     problem.Kind = "preview_topic_not_found"
	KindInvalidUpload     problem.Kind = "preview_invalid_upload"
	KindForbidden         problem.Kind = "preview_forbidden"
	KindInvalidRequest    problem.Kind = "preview_invalid_request"
	KindTestTopicDenied   problem.Kind = "preview_test_topic_denied"
)

// Problem type URIs of live preview (design §7).
const (
	TypeSessionExpired    problem.TypeURI = "https://eventrouter.dev/problems/preview/session-expired"
	TypeInvalidConfig     problem.TypeURI = "https://eventrouter.dev/problems/preview/invalid-config"
	TypeBusy              problem.TypeURI = "https://eventrouter.dev/problems/preview/busy"
	TypeSourceUnavailable problem.TypeURI = "https://eventrouter.dev/problems/preview/source-unavailable"
	TypeTopicNotFound     problem.TypeURI = "https://eventrouter.dev/problems/preview/topic-not-found"
	TypeInvalidUpload     problem.TypeURI = "https://eventrouter.dev/problems/preview/invalid-upload"
	TypeForbidden         problem.TypeURI = "https://eventrouter.dev/problems/preview/forbidden"
	TypeInvalidRequest    problem.TypeURI = "https://eventrouter.dev/problems/preview/invalid-request"
	TypeTestTopicDenied   problem.TypeURI = "https://eventrouter.dev/problems/preview/test-topic-denied"
)

// ProblemEntries maps every preview kind to its problem type and status.
var ProblemEntries = map[problem.Kind]problem.Entry{
	KindSessionExpired:    {Type: TypeSessionExpired, Status: http.StatusGone},
	KindInvalidConfig:     {Type: TypeInvalidConfig, Status: http.StatusUnprocessableEntity},
	KindBusy:              {Type: TypeBusy, Status: http.StatusTooManyRequests},
	KindSourceUnavailable: {Type: TypeSourceUnavailable, Status: http.StatusBadGateway},
	KindTopicNotFound:     {Type: TypeTopicNotFound, Status: http.StatusNotFound},
	KindInvalidUpload:     {Type: TypeInvalidUpload, Status: http.StatusBadRequest},
	KindForbidden:         {Type: TypeForbidden, Status: http.StatusForbidden},
	KindInvalidRequest:    {Type: TypeInvalidRequest, Status: http.StatusBadRequest},
	KindTestTopicDenied:   {Type: TypeTestTopicDenied, Status: http.StatusForbidden},
}

// NewProblemCatalog returns the standard catalog extended with the preview kinds.
func NewProblemCatalog() *problem.Catalog {
	c := problem.NewCatalog()
	for kind, e := range ProblemEntries {
		c.Register(kind, e)
	}
	return c
}

// Detail holds the user-facing messages of preview problems ("what happened, why, what to do").
type Detail string

const (
	DetailSessionExpired   Detail = "This sample session expired or was closed. Reload the samples."
	DetailSessionNotFound  Detail = "This sample session does not exist. Reload the samples."
	DetailBusy             Detail = "Preview is busy, retry in a moment."
	DetailSamplerMissing   Detail = "Live Kafka sampling is not available on this preview service yet. Upload a JSON file to test."
	DetailSourceDown       Detail = "Can't reach the Kafka brokers of this connection. Check the connection's health and retry."
	DetailConnectionGone   Detail = "This connection does not exist anymore. Pick another connection."
	DetailTopicNotFound    Detail = "This topic doesn't exist on the connection. Check the topic name."
	DetailUnknownMode      Detail = "This sample mode is not supported. Use latest, time_range, upload or demo."
	DetailTimeRange        Detail = "A time range needs both from and to, with from before to."
	DetailTopicsRequired   Detail = "Pick at least one topic to sample."
	DetailConnectionNeeded Detail = "Pick a connection to sample from, or upload a JSON file."
	DetailUploadEmpty      Detail = "The upload contains no events. Add at least one event."
	DetailUploadItem       Detail = "This event is not valid JSON or has the wrong shape (expected {key, value, headers, timestamp})."
	DetailUploadLine       Detail = "This line is not valid JSON. Fix it and upload again."
	DetailInvalidConfig    Detail = "The route config has errors. Fix the highlighted fields and run again."
	DetailRunInput         Detail = "Send either a sessionId or inline events to run on."
	DetailTooManyEvents    Detail = "Too many inline events. Send at most 1,000."
	DetailDirective        Detail = "The caller directive is missing or invalid."
	DetailForbidden        Detail = "You can't read samples from this connection. Ask an admin for access."
	DetailBadCursor        Detail = "This page cursor is invalid. Reload the first page."
	DetailBadLimit         Detail = "limit must be a whole number between 1 and 1000."
	DetailUpstream         Detail = "The preview service is unreachable. Retry in a moment."
	DetailUnknownFormat    Detail = "This value format is not supported. Use json, string, bytes, avro or protobuf."
	DetailTestTopicDenied  Detail = "Can't create a test topic on the destination connection. The cluster refused it: check the connection's ACLs, or run the test without writing to a topic."
	DetailTestTopicFailed  Detail = "Can't write to the test topic on the destination connection. Check the connection's health and retry."
	DetailNoDestination    Detail = "The draft has no destination connection to write a test topic to. Pick a destination first."
)

// Sentinel problems returned by the preview domain.
var (
	ErrSessionExpired    = problem.NewError(KindSessionExpired, string(DetailSessionExpired))
	ErrSessionNotFound   = problem.NewError(problem.KindNotFound, string(DetailSessionNotFound))
	ErrBusy              = problem.NewError(KindBusy, string(DetailBusy))
	ErrSamplerMissing    = problem.NewError(KindSourceUnavailable, string(DetailSamplerMissing))
	ErrSourceUnavailable = problem.NewError(KindSourceUnavailable, string(DetailSourceDown))
	ErrConnectionGone    = problem.NewError(problem.KindNotFound, string(DetailConnectionGone))
	ErrTopicNotFound     = problem.NewError(KindTopicNotFound, string(DetailTopicNotFound))
	ErrUnknownMode       = problem.NewError(KindInvalidRequest, string(DetailUnknownMode))
	ErrTimeRange         = problem.NewError(KindInvalidRequest, string(DetailTimeRange))
	ErrTopicsRequired    = problem.NewError(KindInvalidRequest, string(DetailTopicsRequired))
	ErrConnectionNeeded  = problem.NewError(KindInvalidRequest, string(DetailConnectionNeeded))
	ErrUploadEmpty       = problem.NewError(KindInvalidUpload, string(DetailUploadEmpty))
	ErrUploadItem        = problem.NewError(KindInvalidUpload, string(DetailUploadItem))
	ErrUploadLine        = problem.NewError(KindInvalidUpload, string(DetailUploadLine))
	ErrRunInput          = problem.NewError(KindInvalidRequest, string(DetailRunInput))
	ErrTooManyEvents     = problem.NewError(KindInvalidRequest, string(DetailTooManyEvents))
	ErrDirective         = problem.NewError(problem.KindUnauthenticated, string(DetailDirective))
	ErrForbidden         = problem.NewError(KindForbidden, string(DetailForbidden))
	ErrBadCursor         = problem.NewError(KindInvalidRequest, string(DetailBadCursor))
	ErrBadLimit          = problem.NewError(KindInvalidRequest, string(DetailBadLimit))
	ErrUpstream          = problem.NewError(problem.KindUnavailable, string(DetailUpstream))
	ErrUnknownFormat     = problem.NewError(KindInvalidRequest, string(DetailUnknownFormat))
	ErrTestTopicDenied   = problem.NewError(KindTestTopicDenied, string(DetailTestTopicDenied))
	ErrTestTopicFailed   = problem.NewError(KindSourceUnavailable, string(DetailTestTopicFailed))
	ErrNoDestination     = problem.NewError(KindInvalidRequest, string(DetailNoDestination))
)

// RequestField names request members that problems point at.
type RequestField string

const (
	FieldMode         RequestField = "/mode"
	FieldTopics       RequestField = "/topics"
	FieldFrom         RequestField = "/from"
	FieldConnectionID RequestField = "/connectionId"
	FieldEvents       RequestField = "/events"
	FieldUpload       RequestField = "/upload"
	FieldSessionID    RequestField = "/sessionId"
	FieldConfig       RequestField = "/config"
	FieldFormat       RequestField = "/format"
	FieldWriteToTopic RequestField = "/writeToTopic"
)

// Indexed returns the pointer to element i of the field.
func (f RequestField) Indexed(i int) string {
	return string(f) + "/" + strconv.Itoa(i)
}
