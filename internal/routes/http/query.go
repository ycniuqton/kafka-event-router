package routeshttp

import (
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
)

// QueryKey names list query parameters.
type QueryKey string

const (
	QuerySearch     QueryKey = "q"
	QueryStatus     QueryKey = "status"
	QueryConnection QueryKey = "connectionId"
	QueryTag        QueryKey = "tag"
	QueryTeam       QueryKey = "teamId"
)

// ListSeparator splits multi-valued query parameters.
type ListSeparator string

const valueSeparator ListSeparator = ","

// ListValues splits multi-valued query parameters (comma-separated or repeated).
type ListValues struct{}

// Split returns the trimmed, non-empty parts of raw values.
func (ListValues) Split(values []string) []string {
	out := []string{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, string(valueSeparator)) {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Teams returns the owner teams of ?teamId=.
func (v ListValues) Teams(values []string) []routes.TeamID {
	out := []routes.TeamID{}
	for _, t := range v.Split(values) {
		out = append(out, routes.TeamID(t))
	}
	return out
}

// ListQueryParser reads GET /routes filters.
type ListQueryParser struct{}

// Parse validates statuses (comma-separated or repeated) and copies the other filters.
func (ListQueryParser) Parse(r *http.Request) (routes.ListQuery, error) {
	q := r.URL.Query()
	out := routes.ListQuery{Search: q.Get(string(QuerySearch)), ConnectionID: q.Get(string(QueryConnection)),
		Tag: q.Get(string(QueryTag)), TeamIDs: ListValues{}.Teams(q[string(QueryTeam)])}
	known := map[routes.Status]bool{}
	for _, s := range routes.AllStatuses {
		known[s] = true
	}
	for _, part := range (ListValues{}).Split(q[string(QueryStatus)]) {
		s := routes.Status(part)
		if !known[s] {
			return routes.ListQuery{}, api.NewValidationError().Add("/"+string(QueryStatus), string(routes.CodeInvalid),
				"Unknown status. Use draft, running, paused, error or archived.")
		}
		out.Statuses = append(out.Statuses, s)
	}
	return out, nil
}
