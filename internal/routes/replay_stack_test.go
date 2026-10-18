// Stack test of routes#p3-test-replay on the running local stack: a deployed route copies N events, a replay
// without confirm is rejected (422), a confirmed replay resets the consumer group and the destination receives
// the N events again (duplicates). Opt-in: ROUTER_IT_API_URL, ROUTER_IT_KAFKA_BOOTSTRAP, ROUTER_IT_CONNECTION_ID.
package routes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// replayIT holds the knobs of the stack test.
var replayIT = struct {
	EnvAPI, EnvKafka, EnvConnection string
	Events                          int
	Wait, Poll                      time.Duration
	Prefix                          string
}{EnvAPI: "ROUTER_IT_API_URL", EnvKafka: "ROUTER_IT_KAFKA_BOOTSTRAP", EnvConnection: "ROUTER_IT_CONNECTION_ID",
	Events: 5, Wait: 3 * time.Minute, Poll: time.Second, Prefix: "dev-routes-p3-replay-"}

// stackClient calls the local API with a dev-login session and its CSRF token.
type stackClient struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func newStackClient(t *testing.T, base string) *stackClient {
	jar, _ := cookiejar.New(nil)
	c := &stackClient{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	if status, _, _ := c.call(http.MethodPost, "/auth/dev/login", "{}", ""); status != http.StatusNoContent {
		t.Fatalf("dev login: %d", status)
	}
	_, me, _ := c.call(http.MethodGet, "/api/v1/auth/me", "", "")
	c.csrf, _ = me["csrf_token"].(string)
	return c
}

// call sends a JSON request and returns status, decoded object body and ETag.
func (c *stackClient) call(method, path, body, ifMatch string) (int, map[string]any, string) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, resp.Header.Get("ETag")
}

// replayStack is the Kafka side: two fresh topics, a producer and a counter of destination records.
type replayStack struct {
	t       *testing.T
	client  *kgo.Client
	admin   *kadm.Client
	in, out string
}

func newReplayStack(t *testing.T, bootstrap, suffix string) *replayStack {
	cl, err := kgo.NewClient(kgo.SeedBrokers(bootstrap))
	if err != nil {
		t.Fatal(err)
	}
	s := &replayStack{t: t, client: cl, admin: kadm.NewClient(cl), in: replayIT.Prefix + "in-" + suffix, out: replayIT.Prefix + "out-" + suffix}
	if _, err := s.admin.CreateTopics(context.Background(), 1, -1, nil, s.in, s.out); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.admin.DeleteTopics(context.Background(), s.in, s.out)
		cl.Close()
	})
	return s
}

func (s *replayStack) produce(n int) {
	for i := range n {
		rec := &kgo.Record{Topic: s.in, Value: fmt.Appendf(nil, `{"id":%d}`, i)}
		if err := s.client.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
			s.t.Fatal(err)
		}
	}
}

// count reads the destination topic from the start and counts its records.
func (s *replayStack) count() int {
	ends, err := s.admin.ListEndOffsets(context.Background(), s.out)
	if err != nil {
		s.t.Fatal(err)
	}
	total := 0
	ends.Each(func(o kadm.ListedOffset) { total += int(o.Offset) })
	return total
}

// waitFor polls until cond holds or the stack wait elapses.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(replayIT.Wait); time.Now().Before(deadline); time.Sleep(replayIT.Poll) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// deployRoute creates and deploys a route copying in → out from the earliest offset.
func deployRoute(t *testing.T, c *stackClient, s *replayStack, conn, name string) string {
	doc := fmt.Sprintf(`{"apiVersion":"router/v1","name":%q,"source":{"connectionId":%q,"topics":[%q],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"branches":[{"id":"main","destination":{"connectionId":%q,"topic":%q}}],"errorPolicy":{"onError":"skip"}}`, name, conn, s.in, conn, s.out)
	status, created, etag := c.call(http.MethodPost, "/api/v1/routes", fmt.Sprintf(`{"name":%q,"draft":%s}`, name, doc), "")
	if status != http.StatusCreated {
		t.Fatalf("create route: %d %v", status, created)
	}
	id := created["id"].(string)
	t.Cleanup(func() { c.call(http.MethodDelete, "/api/v1/routes/"+id, "", "") })
	if status, body, _ := c.call(http.MethodPost, "/api/v1/routes/"+id+"/deploy", `{"changeNote":"replay stack test"}`, etag); status != http.StatusAccepted {
		t.Fatalf("deploy: %d %v", status, body)
	}
	return id
}

func TestReplayResendsEventsOnLiveStack(t *testing.T) {
	api, bootstrap, conn := os.Getenv(replayIT.EnvAPI), os.Getenv(replayIT.EnvKafka), os.Getenv(replayIT.EnvConnection)
	if api == "" || bootstrap == "" || conn == "" {
		t.Skip("set ROUTER_IT_API_URL, ROUTER_IT_KAFKA_BOOTSTRAP and ROUTER_IT_CONNECTION_ID to replay on the local stack")
	}
	suffix := uuid.NewString()[:8]
	c, s := newStackClient(t, api), newReplayStack(t, bootstrap, suffix)
	start := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	s.produce(replayIT.Events)
	id := deployRoute(t, c, s, conn, replayIT.Prefix+suffix)
	waitFor(t, "the first copy", func() bool { return s.count() >= replayIT.Events })

	if status, body, _ := c.call(http.MethodPost, "/api/v1/routes/"+id+"/replay", fmt.Sprintf(`{"from":%q}`, start), ""); status != http.StatusUnprocessableEntity {
		t.Fatalf("replay without confirm: %d %v", status, body)
	}
	status, rp, _ := c.call(http.MethodPost, "/api/v1/routes/"+id+"/replay", fmt.Sprintf(`{"from":%q,"confirm":true}`, start), "")
	if status != http.StatusAccepted || rp["status"] != "pending" {
		t.Fatalf("replay: %d %v", status, rp)
	}
	waitFor(t, "the replay to finish", func() bool {
		_, cur, _ := c.call(http.MethodGet, fmt.Sprintf("/api/v1/routes/%s/replays/%s", id, rp["replayId"]), "", "")
		if cur["status"] == "failed" {
			t.Fatalf("replay failed: %v", cur["error"])
		}
		return cur["status"] == "done"
	})
	waitFor(t, "the duplicates", func() bool { return s.count() >= 2*replayIT.Events })
}
