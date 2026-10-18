package dlq

import (
	"strconv"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Domain errors; each is a classified problem so handlers return them unchanged (design §4.8).
var (
	ErrRouteNotFound    = problem.NewError(problem.KindNotFound, "The route does not exist or you cannot read it.")
	ErrRecordNotFound   = problem.NewError(problem.KindNotFound, "This dead letter does not exist in the route's DLQ topics.")
	ErrRecordExpired    = problem.NewError(problem.KindNotFound, "This dead letter expired by DLQ retention.")
	ErrInvalidID        = problem.NewError(problem.KindMalformedBody, "The dead-letter id is malformed. Use an id from the list.")
	ErrInvalidCursor    = problem.NewError(problem.KindMalformedBody, "The cursor is malformed or from another query. Restart from the first page.")
	ErrInvalidFilter    = problem.NewError(problem.KindMalformedBody, "The filter value is invalid. Check the expected format.")
	ErrSelectionShape   = problem.NewError(problem.KindMalformedBody, "Send exactly one of ids or match.")
	ErrSelectionTooBig  = problem.NewError(problem.KindValidation, "Too many events for one request. Select fewer events or start a background job for this filter.")
	ErrExportTooBig     = problem.NewError(problem.KindValidation, "Too many events for one export. Narrow the filter.")
	ErrCountMismatch    = problem.NewError(problem.KindConflict, "The list changed since you loaded it. Reload and try again.")
	ErrNoActiveVersion  = problem.NewError(problem.KindConflict, "Deploy the route before retrying: it has no active version.")
	ErrUnknownFormat    = problem.NewError(problem.KindMalformedBody, "Unknown export format. Use json or ndjson.")
	ErrUpstream         = problem.NewError(problem.KindUnavailable, "The DLQ cluster could not be reached. Check the connection and try again.")
	ErrStateUnavailable = problem.NewError(problem.KindUnavailable, "Resolved state is still loading. Try again in a few seconds.")
	ErrJobAction        = problem.NewError(problem.KindMalformedBody, "Background jobs support retry and discard.")
	ErrStudioDown       = problem.NewError(problem.KindUnavailable, "Preview service unavailable, try again.")
)

// RequestPath names JSON Pointer paths used in problems.
type RequestPath string

const (
	PathRoot     RequestPath = ""
	PathIDs      RequestPath = "/ids"
	PathCursor   RequestPath = "/cursor"
	PathDLQID    RequestPath = "/dlqId"
	PathFormat   RequestPath = "/format"
	PathQuery    RequestPath = "/query"
	PathActive   RequestPath = "/activeVersion"
	PathSince    RequestPath = "/query/since"
	PathUntil    RequestPath = "/query/until"
	PathLimit    RequestPath = "/query/limit"
	PathErrType  RequestPath = "/query/errorType"
	PathRouteIDs RequestPath = "/query/routeId"
	PathVersion  RequestPath = "/query/version"
	PathAction   RequestPath = "/action"
	PathMatchVer RequestPath = "/match/version"
)

// Indexed returns the pointer of item i below p.
func (p RequestPath) Indexed(i int) string {
	return string(p) + "/" + strconv.Itoa(i)
}
