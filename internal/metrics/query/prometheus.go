package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/httpserver"
)

// PromAPIPath names the Prometheus HTTP API v1 endpoints (VictoriaMetrics serves the same paths).
type PromAPIPath string

const (
	PathInstant PromAPIPath = "/api/v1/query"
	PathRange   PromAPIPath = "/api/v1/query_range"
)

// PromParam names the form parameters of the query API.
type PromParam string

const (
	ParamQuery PromParam = "query"
	ParamTime  PromParam = "time"
	ParamStart PromParam = "start"
	ParamEnd   PromParam = "end"
	ParamStep  PromParam = "step"
)

// PromLimit bounds a response body read from the metrics store.
type PromLimit int64

const MaxResponseBytes PromLimit = 32 << 20

// FormMediaType is the request body type of query calls.
type FormMediaType string

const MediaTypeForm FormMediaType = "application/x-www-form-urlencoded"

// ErrNoURL is returned when the Prometheus backend is selected without ROUTER_METRICS_URL.
var ErrNoURL = errors.New("metrics: prometheus backend needs a URL")

// PrometheusQuerier queries the Prometheus HTTP API; every failure becomes an UnavailableError.
type PrometheusQuerier struct {
	base    *url.URL
	client  *http.Client
	timeout time.Duration
	now     func() time.Time
	decoder PromResponseDecoder
}

// NewPrometheusQuerier builds the querier from config.
func NewPrometheusQuerier(cfg BackendConfig) (*PrometheusQuerier, error) {
	if cfg.URL == "" {
		return nil, ErrNoURL
	}
	base, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil {
		return nil, fmt.Errorf("metrics: invalid URL: %w", err)
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{}
	}
	return &PrometheusQuerier{base: base, client: client, timeout: cfg.Timeout, now: time.Now}, nil
}

// Kind implements Querier.
func (q *PrometheusQuerier) Kind() BackendKind { return BackendPrometheus }

// Instant evaluates e at the current time.
func (q *PrometheusQuerier) Instant(ctx context.Context, e Expr) (Vector, error) {
	form := url.Values{string(ParamQuery): {string(e)}, string(ParamTime): {q.unix(q.now())}}
	body, err := q.post(ctx, PathInstant, form)
	if err != nil {
		return nil, err
	}
	return q.decoder.Vector(body)
}

// Range evaluates e over w.
func (q *PrometheusQuerier) Range(ctx context.Context, e Expr, w Window) ([]Series, error) {
	form := url.Values{string(ParamQuery): {string(e)}, string(ParamStart): {q.unix(w.Start)},
		string(ParamEnd): {q.unix(w.End)}, string(ParamStep): {strconv.FormatFloat(w.Step.Seconds(), 'f', -1, 64)}}
	body, err := q.post(ctx, PathRange, form)
	if err != nil {
		return nil, err
	}
	return q.decoder.Matrix(body)
}

func (q *PrometheusQuerier) unix(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

// post sends one form request under the configured timeout and returns the body of a 2xx/4xx answer.
func (q *PrometheusQuerier) post(ctx context.Context, path PromAPIPath, form url.Values) ([]byte, error) {
	if q.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.base.String()+string(path), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, Unavailable(err)
	}
	req.Header.Set(string(httpserver.HeaderContentType), string(MediaTypeForm))
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, Unavailable(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBytes)))
	if err != nil {
		return nil, Unavailable(err)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, Unavailable(fmt.Errorf("metrics store status %d", resp.StatusCode))
	}
	return body, nil
}
