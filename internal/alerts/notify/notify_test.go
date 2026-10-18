package notify

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/alertstest"
	"github.com/eventrouter/router/internal/secrets"
)

var (
	ruleID  = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	routeID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	fired   = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
)

func sample() Notification {
	v := 25000.0
	return Notification{Type: EventFired, RuleID: ruleID, RuleName: "Lag high", Kind: alerts.KindLagAbove, Threshold: 20000,
		Window: 10, RouteID: routeID, RouteName: "orders", Value: &v, FiredAt: fired, Link: "https://router.example/routes/" + routeID.String()}
}

type capture struct {
	mu      sync.Mutex
	bodies  [][]byte
	headers []http.Header
	status  int
}

func (c *capture) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies, c.headers = append(c.bodies, b), append(c.headers, r.Header.Clone())
		status := c.status
		c.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(s.Close)
	return s
}

func poster() JSONPoster { return JSONPoster{Client: &http.Client{Timeout: 2 * time.Second}} }

func TestWebhookPayloadGoldenAndSignature(t *testing.T) {
	c := &capture{}
	srv := c.server(t)
	key := []byte("signing-key")
	now := func() time.Time { return time.Unix(1791966968, 0) }
	n := WebhookNotifier{Poster: poster(), SigningKey: key, Now: now}
	if err := n.Send(context.Background(), sample(), Destination{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	want := `{"type":"alert.fired","ruleId":"11111111-1111-4111-8111-111111111111","ruleName":"Lag high","kind":"lag_above","threshold":20000,"windowMinutes":10,"routeId":"22222222-2222-4222-8222-222222222222","routeName":"orders","value":25000,"firedAt":"2026-10-14T09:00:00Z","link":"https://router.example/routes/22222222-2222-4222-8222-222222222222"}`
	if string(c.bodies[0]) != want {
		t.Fatalf("payload:\n got %s\nwant %s", c.bodies[0], want)
	}
	h := c.headers[0]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(h.Get(string(HeaderTimestamp)) + "."))
	mac.Write(c.bodies[0])
	if h.Get(string(HeaderSignature)) != "sha256="+hex.EncodeToString(mac.Sum(nil)) || h.Get(string(HeaderTimestamp)) != "1791966968" {
		t.Fatalf("signature headers: %v", h)
	}
	if h.Get(string(HeaderEvent)) != string(EventFired) {
		t.Fatalf("event header: %v", h)
	}
}

func TestWebhookWithoutKeyIsUnsignedAndNon2xxFails(t *testing.T) {
	c := &capture{status: http.StatusBadGateway}
	srv := c.server(t)
	err := WebhookNotifier{Poster: poster(), Now: time.Now}.Send(context.Background(), sample(), Destination{URL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("want HTTP 502 error, got %v", err)
	}
	if c.headers[0].Get(string(HeaderSignature)) != "" {
		t.Fatal("no key → no signature")
	}
}

func TestSlackAndTeamsPayloads(t *testing.T) {
	c := &capture{}
	srv := c.server(t)
	if err := (SlackNotifier{Poster: poster()}).Send(context.Background(), sample(), Destination{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	var slack SlackPayload
	_ = json.Unmarshal(c.bodies[0], &slack)
	wantSlack := "*[FIRING] Lag high · orders*\nConsumer lag stayed above 20000 for 10 min\nCurrent value: 25000\nOpen the route: https://router.example/routes/" + routeID.String()
	if slack.Text != wantSlack {
		t.Fatalf("slack:\n got %q\nwant %q", slack.Text, wantSlack)
	}
	resolved := sample()
	resolved.Type = EventResolved
	if err := (TeamsNotifier{Poster: poster()}).Send(context.Background(), resolved, Destination{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	var card TeamsCard
	_ = json.Unmarshal(c.bodies[1], &card)
	if card.Type != "MessageCard" || card.ThemeColor != string(ColorResolved) || card.Title != "[RESOLVED] Lag high · orders" ||
		!strings.Contains(card.Text, "back to normal") {
		t.Fatalf("teams card: %+v", card)
	}
}

// fakeSMTP is a minimal SMTP server that accepts one message and records DATA.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	data []string
	rcpt []string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.session(conn)
	}
}

func (f *fakeSMTP) session(conn net.Conn) {
	defer conn.Close()
	r, w := bufio.NewReader(conn), bufio.NewWriter(conn)
	reply := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }
	reply("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 fake")
		case strings.HasPrefix(cmd, "RCPT TO"):
			f.mu.Lock()
			f.rcpt = append(f.rcpt, strings.TrimSpace(line))
			f.mu.Unlock()
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go")
			f.readData(r)
			reply("250 queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (f *fakeSMTP) readData(r *bufio.Reader) {
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil || line == ".\r\n" {
			break
		}
		b.WriteString(line)
	}
	f.mu.Lock()
	f.data = append(f.data, b.String())
	f.mu.Unlock()
}

func TestEmailOverFakeSMTP(t *testing.T) {
	srv := newFakeSMTP(t)
	host, port, _ := net.SplitHostPort(srv.ln.Addr().String())
	p, _ := strconv.Atoi(port)
	n := EmailNotifier{SMTP: SMTPSettings{Host: host, Port: p, From: "router@example.com"}, Mailer: smtp.SendMail}
	if err := n.Send(context.Background(), sample(), Destination{Recipients: []string{"a@example.com", "b@example.com"}}); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.rcpt) != 2 || len(srv.data) != 1 {
		t.Fatalf("rcpt=%v data=%d", srv.rcpt, len(srv.data))
	}
	msg := srv.data[0]
	for _, want := range []string{"Subject: [FIRING] Lag high · orders", "To: a@example.com, b@example.com", "Current value: 25000"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	if err := (EmailNotifier{Mailer: smtp.SendMail}).Send(context.Background(), sample(), Destination{Recipients: []string{"a@b.c"}}); err != ErrSMTPNotConfigured {
		t.Fatalf("unconfigured: %v", err)
	}
}

type memSecrets map[string]string

func (m memSecrets) Get(_ context.Context, ref secrets.Ref) ([]byte, error) {
	return []byte(m[string(ref)]), nil
}

type countDeliveries struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *countDeliveries) Delivered(ch alerts.ChannelType, r DeliveryResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[string(ch)+"/"+string(r)]++
}

func TestDispatcherRetriesThreeTicksThenGivesUp(t *testing.T) {
	c := &capture{status: http.StatusInternalServerError}
	srv := c.server(t)
	rules := &alertstest.Rules{}
	rule, _ := rules.Insert(context.Background(), alerts.AlertRule{RouteID: routeID, Name: "r", Kind: alerts.KindThroughputZero, Window: 1,
		Channels: []alerts.ChannelRef{{Type: alerts.ChannelWebhook, SecretRef: "s1"}, {Type: alerts.ChannelSlack, SecretRef: "s2"}}})
	evs := &alertstest.Events{}
	ev, _, _ := evs.Fire(context.Background(), alerts.AlertEvent{RuleID: rule.ID, RouteID: routeID, FiredAt: fired})
	counts := &countDeliveries{}
	d := &NotificationDispatcher{Notifiers: NewNotifierManager([]alerts.ChannelType{alerts.ChannelWebhook}, WebhookNotifier{Poster: poster(), Now: time.Now},
		SlackNotifier{Poster: poster()}), Secrets: memSecrets{"s1": srv.URL, "s2": srv.URL}, Events: evs, Rules: rules,
		Routes: alertstest.Routes{routeID: {ID: routeID, Name: "orders"}}, Counter: counts, MaxAttempts: 3, Now: time.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for i := 0; i < 5; i++ {
		if err := d.DispatchPending(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	got := evs.Rows[0]
	if len(c.bodies) != 3 || got.NotifyAttempts != 3 || got.NotifyPending != nil || got.ID != ev.ID {
		t.Fatalf("posts=%d attempts=%d pending=%v", len(c.bodies), got.NotifyAttempts, got.NotifyPending)
	}
	if !strings.Contains(got.NotifyError, "slack: channel slack disabled") || !strings.Contains(got.NotifyError, "webhook: HTTP 500") {
		t.Fatalf("notify_error: %s", got.NotifyError)
	}
	if counts.n["webhook/error"] != 3 || counts.n["slack/disabled"] != 3 {
		t.Fatalf("counter: %v", counts.n)
	}
}

func TestDispatcherMarksNotifiedOnce(t *testing.T) {
	c := &capture{}
	srv := c.server(t)
	rules := &alertstest.Rules{}
	rule, _ := rules.Insert(context.Background(), alerts.AlertRule{RouteID: routeID, Name: "r", Kind: alerts.KindLagAbove, Window: 1,
		Channels: []alerts.ChannelRef{{Type: alerts.ChannelWebhook, SecretRef: "s1"}}})
	evs := &alertstest.Events{}
	_, _, _ = evs.Fire(context.Background(), alerts.AlertEvent{RuleID: rule.ID, RouteID: routeID, FiredAt: fired})
	d := &NotificationDispatcher{Notifiers: NewNotifierManager(nil, WebhookNotifier{Poster: poster(), Now: time.Now}),
		Secrets: memSecrets{"s1": srv.URL}, Events: evs, Rules: rules, Routes: alertstest.Routes{routeID: {ID: routeID}},
		Counter: &countDeliveries{}, MaxAttempts: 3, Now: time.Now, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for i := 0; i < 3; i++ {
		_ = d.DispatchPending(context.Background())
	}
	if len(c.bodies) != 1 || evs.Rows[0].NotifiedAt == nil {
		t.Fatalf("posts=%d notified=%v", len(c.bodies), evs.Rows[0].NotifiedAt)
	}
	if (LinkBuilder{PublicURL: "https://x/"}).Route("r1") != "https://x/routes/r1" || (LinkBuilder{}).Route("r1") != "" {
		t.Fatal("links")
	}
}
