// Package gateway is the API-side entry to live preview: it authenticates and authorizes the
// caller, signs a caller directive, forwards to the internal preview service and audits sample loads.
package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/preview"
)

// ClientPolicy bounds calls to the preview service (run deadline plus transfer slack).
type ClientPolicy time.Duration

const UpstreamTimeout ClientPolicy = ClientPolicy(30 * time.Second)

// MaxUpstreamBody caps the size of a preview response read by the gateway.
type MaxUpstreamBody int64

const UpstreamBodyLimit MaxUpstreamBody = 64 << 20

// Upstream is a preview service response relayed verbatim to the browser.
type Upstream struct {
	Status      int
	ContentType string
	Body        []byte
}

// PreviewClient calls /internal/v1/preview/* on ROUTER_PREVIEW_URL with the signed directive.
type PreviewClient struct {
	baseURL string
	http    *http.Client
	codec   preview.DirectiveCodec
}

// NewPreviewClient builds a client.
func NewPreviewClient(baseURL string, httpClient *http.Client, codec preview.DirectiveCodec) *PreviewClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(UpstreamTimeout)}
	}
	return &PreviewClient{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, codec: codec}
}

// Forward sends method + internal path (with query) and body for caller; transport failures are 503.
func (c *PreviewClient) Forward(ctx context.Context, method, path string, body []byte, caller preview.Caller) (Upstream, error) {
	directive, err := c.codec.Encode(caller)
	if err != nil {
		return Upstream{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return Upstream{}, err
	}
	req.Header.Set(string(preview.HeaderCaller), directive)
	if body != nil {
		req.Header.Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Upstream{}, preview.ErrUpstream.Wrap(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(UpstreamBodyLimit)))
	if err != nil {
		return Upstream{}, preview.ErrUpstream.Wrap(err)
	}
	return Upstream{Status: resp.StatusCode, ContentType: resp.Header.Get(string(httpserver.HeaderContentType)), Body: data}, nil
}

// Write relays an upstream response.
func (u Upstream) Write(w http.ResponseWriter) error {
	if u.ContentType != "" {
		w.Header().Set(string(httpserver.HeaderContentType), u.ContentType)
	}
	w.WriteHeader(u.Status)
	_, err := w.Write(u.Body)
	return err
}

// OK reports a 2xx status.
func (u Upstream) OK() bool {
	return u.Status >= http.StatusOK && u.Status < http.StatusMultipleChoices
}
