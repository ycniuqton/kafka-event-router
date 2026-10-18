package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/preview"
)

// RunSource is where the samples of a run come from; the masking decision follows its connection.
// Failure is the preview service's answer when the session could not be read (relayed as is).
type RunSource struct {
	ConnectionID string
	Failure      *Upstream
}

// Relay writes the session lookup failure, or returns err.
func (s RunSource) Relay(w http.ResponseWriter, err error) error {
	if err != nil || s.Failure == nil {
		return err
	}
	return s.Failure.Write(w)
}

// RunSourceResolver finds a run's sample source: the session it runs on (or copied its inline events from),
// read from the preview service, else the draft's source connection.
type RunSourceResolver struct {
	Client     *PreviewClient
	Authorizer PreviewAuthorizer
}

// Of resolves the source of the run probed from the request body.
func (r RunSourceResolver) Of(ctx context.Context, p auth.Principal, probe RunProbe) (RunSource, error) {
	if probe.Session == "" {
		return RunSource{ConnectionID: probe.Refs.Source}, nil
	}
	path := string(preview.PrefixInternal) + strings.Replace(string(preview.EndpointSession), "{"+string(preview.ParamSessionID)+"}", url.PathEscape(string(probe.Session)), 1)
	up, err := r.Client.Forward(ctx, http.MethodGet, path, nil, r.Authorizer.Identity(p))
	if err != nil {
		return RunSource{}, err
	}
	if !up.OK() {
		return RunSource{Failure: &up}, nil
	}
	var src preview.SessionSourceDTO
	if err := json.Unmarshal(up.Body, &src); err != nil {
		return RunSource{}, preview.ErrUpstream.Wrap(err)
	}
	return RunSource{ConnectionID: src.ConnectionID}, nil
}
