package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HeaderContentType names the request body media type header.
const HeaderContentType Header = "Content-Type"

// MediaType names body media types used on the internal API.
type MediaType string

const (
	MediaJSON        MediaType = "application/json"
	MediaEventStream MediaType = "text/event-stream"
)

// HeaderAccept names the accepted response media type header.
const HeaderAccept Header = "Accept"

// Request is one JSON call to the internal listener; Body is JSON-encoded when set.
type Request struct {
	Method  string
	Path    string
	Body    any
	Headers map[Header]string
}

// Response is the status and headers of a call; the JSON body was decoded into the caller's target.
type Response struct {
	Status int
	Header http.Header
}

// Call sends req with the workload credential; 2xx bodies are decoded into out (nil skips), 304 is
// returned as-is and every other status becomes a *StatusError.
func (c *Client) Call(ctx context.Context, req Request, out any) (Response, error) {
	httpReq, err := c.newRequest(ctx, req)
	if err != nil {
		return Response{}, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("controlplane: %w", err)
	}
	defer resp.Body.Close()
	res := Response{Status: resp.StatusCode, Header: resp.Header}
	if resp.StatusCode == http.StatusNotModified {
		return res, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return res, &StatusError{Status: resp.StatusCode, Path: req.Path}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return res, nil
	}
	return res, json.NewDecoder(resp.Body).Decode(out)
}

// Stream opens a long-lived GET (server-sent events) without the per-call timeout; the caller closes it.
func (c *Client) Stream(ctx context.Context, path string) (io.ReadCloser, error) {
	httpReq, err := c.newRequest(ctx, Request{Method: http.MethodGet, Path: path, Headers: map[Header]string{HeaderAccept: string(MediaEventStream)}})
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Transport: c.http.Transport}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("controlplane: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &StatusError{Status: resp.StatusCode, Path: path}
	}
	return resp.Body, nil
}

func (c *Client) newRequest(ctx context.Context, req Request) (*http.Request, error) {
	token, err := c.credential.Token(ctx)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, c.baseURL+req.Path, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(string(HeaderAuthorization), string(SchemeBearer)+token)
	if req.Body != nil {
		httpReq.Header.Set(string(HeaderContentType), string(MediaJSON))
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(string(k), v)
	}
	return httpReq, nil
}
