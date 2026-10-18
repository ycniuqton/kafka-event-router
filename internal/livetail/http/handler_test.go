package livetailhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/livetail"
	livetailhttp "github.com/eventrouter/router/internal/livetail/http"
	"github.com/eventrouter/router/internal/livetail/workertap"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/engine"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// routeDir serves fixed routes.
type routeDir map[string]livetail.TailRoute

func (d routeDir) TailRoute(_ context.Context, id string) (livetail.TailRoute, error) {
	r, ok := d[id]
	if !ok {
		return livetail.TailRoute{}, routes.ErrRouteNotFound
	}
	return r, nil
}

// fixedVisibility masks per the test.
type fixedVisibility bool

func (v fixedVisibility) Masked(context.Context, string) bool { return bool(v) }

// auditLog captures livetail.opened calls.
type auditLog struct {
	mu     sync.Mutex
	masked []bool
}

func (a *auditLog) Opened(_ context.Context, _ auth.Principal, _ livetail.TailRoute, masked bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.masked = append(a.masked, masked)
	return nil
}

type harness struct {
	mr    *miniredis.Miniredis
	tap   *livetail.RedisTap
	srv   *httptest.Server
	audit *auditLog
}

const (
	runningRoute = "11111111-1111-1111-1111-111111111111"
	pausedRoute  = "22222222-2222-2222-2222-222222222222"
)

func newHarness(t *testing.T, ctx context.Context) *harness {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	limits := livetail.DefaultTailLimits()
	limits.FlagCacheTTL, limits.ReadBlock = 10*time.Millisecond, 200*time.Millisecond
	tap := livetail.NewRedisTap(client, limits, nil, quiet)
	go tap.Run(ctx)
	audit := &auditLog{}
	dir := routeDir{runningRoute: {ID: runningRoute, Name: "orders", Status: routes.StatusRunning, SourceConnectionID: "c1"},
		pausedRoute: {ID: pausedRoute, Name: "paused", Status: routes.StatusPaused, SourceConnectionID: "c1"}}
	responder := api.NewResponder(problem.NewCatalog(), quiet)
	up := api.NewWSUpgrader(api.NewWSOriginPolicy(nil), time.Minute, responder)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			p := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}
			next.ServeHTTP(w, req.WithContext(auth.WithPrincipal(req.Context(), p)))
		})
	})
	api.NewMounter(responder, nil, nil).Mount(r, livetailhttp.NewModule(up, livetailhttp.Deps{Routes: dir,
		Store: livetail.RedisTailStore{Client: client, Limits: limits}, Visibility: fixedVisibility(true),
		Masker: livetail.NewTypePreservingTailMasker(), Audit: audit, Limits: limits, Log: quiet}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &harness{mr: mr, tap: tap, srv: srv, audit: audit}
}

func (h *harness) dial(t *testing.T, ctx context.Context, route string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + strings.Replace(string(livetailhttp.PathTail), "{id}", route, 1)
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func readFrame(ctx context.Context, c *websocket.Conn) (livetail.Frame, error) {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, data, err := c.Read(rctx)
	if err != nil {
		return livetail.Frame{}, err
	}
	var f livetail.Frame
	return f, json.Unmarshal(data, &f)
}

func waitActive(t *testing.T, tap livetail.Tap, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tap.Active(runningRoute) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tap Active never became %v", want)
}

func runnerEvent(i int) runner.TailEvent {
	value := []byte(`{"email":"ann@example.com","n":` + string(rune('0'+i%10)) + `}`)
	return runner.TailEvent{At: time.Now(), Offset: int64(i), Outcome: domain.OutcomeProduced,
		Input: engine.RawRecord{Topic: "orders", Offset: int64(i), Value: value}}
}

func TestTailStreamsRateLimitedMaskedRecordsAndStopsAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newHarness(t, ctx)
	c := h.dial(t, ctx, runningRoute)
	if f, err := readFrame(ctx, c); err != nil || f.State != livetail.StateLive || !f.Masked {
		t.Fatalf("hello status %+v %v", f, err)
	}
	waitActive(t, h.tap, true)
	worker := workertap.RunnerTap{Tap: h.tap}
	for i := 0; i < 50; i++ {
		worker.Offer(runningRoute, runnerEvent(i))
	}
	records, throttled := 0, 0
	for {
		f, err := readFrame(ctx, c)
		if err != nil {
			break
		}
		if f.Type == livetail.FrameStatus && f.State == livetail.StateThrottled {
			throttled++
			continue
		}
		records++
		if email := f.Record.Input.Value.(map[string]any)["email"]; email == "ann@example.com" || !f.Masked {
			t.Fatalf("record not masked: %v", email)
		}
		if records+throttled >= 21 {
			break
		}
	}
	if records > livetail.DefaultTailLimits().Burst || throttled != 1 {
		t.Fatalf("records=%d throttled=%d", records, throttled)
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	h.mr.FastForward(livetail.DefaultTailLimits().FlagTTL + time.Second)
	waitActive(t, h.tap, false)
	if len(h.audit.masked) != 1 || !h.audit.masked[0] {
		t.Fatalf("audit %v", h.audit.masked)
	}
}

func TestTailClosesWhenRouteNotRunningOrRedisDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newHarness(t, ctx)
	c := h.dial(t, ctx, pausedRoute)
	if _, err := readFrame(ctx, c); websocket.CloseStatus(err) != websocket.StatusCode(livetailhttp.CloseRouteNotRunning) {
		t.Fatalf("paused route close = %v", err)
	}
	h.mr.Close()
	c = h.dial(t, ctx, runningRoute)
	if _, err := readFrame(ctx, c); websocket.CloseStatus(err) != websocket.StatusCode(livetailhttp.CloseUnavailable) {
		t.Fatalf("redis down close = %v", err)
	}
}
