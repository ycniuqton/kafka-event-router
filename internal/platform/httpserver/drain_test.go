package httpserver

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestShutdownEndsStreams checks that a stream bound by StreamContext ends when Shutdown starts, so the
// graceful shutdown finishes well before its timeout.
func TestShutdownEndsStreams(t *testing.T) {
	started := make(chan struct{})
	ended := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, stop := StreamContext{}.Of(r.Context())
		defer stop()
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		close(started)
		<-ctx.Done()
		close(ended)
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	NewDrain().Install(srv)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case <-ended:
	default:
		t.Fatal("stream still open after shutdown")
	}
}

// TestStreamContextWithoutServer keeps a plain request context usable.
func TestStreamContextWithoutServer(t *testing.T) {
	ctx, stop := StreamContext{}.Of(context.Background())
	if ctx.Err() != nil {
		t.Fatal("context ended early")
	}
	stop()
	if ctx.Err() == nil {
		t.Fatal("stop did not end the context")
	}
}
