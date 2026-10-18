package preview

// Endpoint is a preview route relative to its listener prefix (same on the API and the service).
type Endpoint string

const (
	EndpointSamples       Endpoint = "/samples"
	EndpointSession       Endpoint = "/sessions/{id}"
	EndpointSessionEvents Endpoint = "/sessions/{id}/events"
	EndpointSessionSchema Endpoint = "/sessions/{id}/schema"
	EndpointRun           Endpoint = "/run"
	EndpointTestRuns      Endpoint = "/test-runs"
)

// PathPrefix is where preview endpoints are mounted: publicly on the API, internally on the service.
type PathPrefix string

const (
	PrefixPublic   PathPrefix = "/api/v1/preview"
	PrefixInternal PathPrefix = "/internal/v1/preview"
)

// Pattern returns the chi pattern of e under the prefix.
func (p PathPrefix) Pattern(e Endpoint) string {
	return string(p) + string(e)
}

// PathParam names route parameters.
type PathParam string

const ParamSessionID PathParam = "id"

// QueryParam names query parameters of the events listing.
type QueryParam string

const (
	QueryCursor QueryParam = "cursor"
	QueryLimit  QueryParam = "limit"
	QueryText   QueryParam = "q"
)
