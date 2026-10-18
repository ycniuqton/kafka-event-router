// Package platform drives the router's public API for the stress runner: sign-in, connections, routes (create,
// deploy, wait until running, archive) — mirroring test/acceptance/lib (CSRF double submit, If-Match ETags).
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Wire holds the HTTP names the API expects.
var Wire = struct {
	CSRFHeader, CSRFCookie, IfMatch, ETag, Auth, ContentType, JSON string
	Timeout                                                        time.Duration
}{
	CSRFHeader: "X-CSRF-Token", CSRFCookie: "er_csrf", IfMatch: "If-Match", ETag: "Etag", Auth: "Authorization",
	ContentType: "Content-Type", JSON: "application/json", Timeout: 60 * time.Second,
}

// Response is one API answer.
type Response struct {
	Status int
	Body   []byte
	ETag   string
}

// OK reports a 2xx status.
func (r Response) OK() bool {
	return r.Status >= 200 && r.Status < 300
}

// Decode unmarshals the body into v.
func (r Response) Decode(v any) error {
	return json.Unmarshal(r.Body, v)
}

// Err returns an error describing a non-2xx answer (body truncated), or nil.
func (r Response) Err(what string) error {
	if r.OK() {
		return nil
	}
	body := r.Body
	if len(body) > 600 {
		body = body[:600]
	}
	return fmt.Errorf("%s: HTTP %d %s", what, r.Status, body)
}

// Client is a JSON API session (cookie jar + CSRF, or bearer token).
type Client struct {
	base   *url.URL
	http   *http.Client
	bearer string
}

// NewClient returns an unauthenticated session for baseURL.
func NewClient(baseURL string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Timeout: Wire.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{base: u, http: hc}, nil
}

// Do sends method path with an optional JSON body and If-Match; rate-limited answers (429) are retried after
// their Retry-After, so bulk provisioning runs at the API's per-user rate instead of failing.
func (c *Client) Do(ctx context.Context, method, path string, body any, ifMatch string) (Response, error) {
	var raw []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return Response{}, err
		}
		raw = b
	}
	for attempt := 0; ; attempt++ {
		res, wait, err := c.send(ctx, method, path, raw, body != nil, ifMatch)
		if err != nil || res.Status != http.StatusTooManyRequests || attempt >= RateLimitRetry.Attempts {
			return res, err
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// RateLimitRetry bounds the retries of rate-limited requests.
var RateLimitRetry = struct {
	Attempts int
	Default  time.Duration
}{Attempts: 120, Default: time.Second}

// send performs one request and returns the answer and the server's Retry-After.
func (c *Client) send(ctx context.Context, method, path string, raw []byte, hasBody bool, ifMatch string) (Response, time.Duration, error) {
	var rd io.Reader
	if hasBody {
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL(path).String(), rd)
	if err != nil {
		return Response{}, 0, err
	}
	c.decorate(req, hasBody, ifMatch)
	res, err := c.http.Do(req)
	if err != nil {
		return Response{}, 0, err
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	return Response{Status: res.StatusCode, Body: out, ETag: res.Header.Get(Wire.ETag)}, c.retryAfter(res.Header), err
}

func (c *Client) retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || secs <= 0 {
		return RateLimitRetry.Default
	}
	return time.Duration(secs) * time.Second
}

// Get is Do(GET).
func (c *Client) Get(ctx context.Context, path string) (Response, error) {
	return c.Do(ctx, http.MethodGet, path, nil, "")
}

// WithETag sends a write carrying the current ETag of etagOf (GET first), like the acceptance suite.
func (c *Client) WithETag(ctx context.Context, method, path string, body any, etagOf string) (Response, error) {
	cur, err := c.Get(ctx, etagOf)
	if err != nil {
		return Response{}, err
	}
	return c.Do(ctx, method, path, body, cur.ETag)
}

// HTTP exposes the underlying client (login flows follow redirects by hand).
func (c *Client) HTTP() *http.Client {
	return c.http
}

// URL resolves a path (with optional ?query) against the base URL.
func (c *Client) URL(path string) *url.URL {
	p, q, _ := strings.Cut(path, "?")
	u := c.base.JoinPath(p)
	u.RawQuery = q
	return u
}

// SetBearer switches the session to API-token authentication.
func (c *Client) SetBearer(token string) {
	c.bearer = token
}

func (c *Client) decorate(req *http.Request, hasBody bool, ifMatch string) {
	if hasBody {
		req.Header.Set(Wire.ContentType, Wire.JSON)
	}
	if ifMatch != "" {
		req.Header.Set(Wire.IfMatch, ifMatch)
	}
	if c.bearer != "" {
		req.Header.Set(Wire.Auth, "Bearer "+c.bearer)
		return
	}
	for _, ck := range c.http.Jar.Cookies(c.base) {
		if ck.Name == Wire.CSRFCookie {
			req.Header.Set(Wire.CSRFHeader, ck.Value)
		}
	}
}
