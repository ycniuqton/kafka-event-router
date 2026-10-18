package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientRetriesRateLimitedRequests(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	saved := RateLimitRetry.Default
	RateLimitRetry.Default = 0
	defer func() { RateLimitRetry.Default = saved }()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(context.Background(), http.MethodPost, "/api/v1/routes", map[string]string{"name": "stress-x"}, "")
	if err != nil || res.Status != http.StatusCreated || calls.Load() != 3 {
		t.Fatalf("status %d after %d calls, err %v", res.Status, calls.Load(), err)
	}
}
