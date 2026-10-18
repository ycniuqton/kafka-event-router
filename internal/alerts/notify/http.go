package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
)

// HTTPHeader names headers of outgoing notifications.
type HTTPHeader string

const (
	HeaderContentType HTTPHeader = "Content-Type"
	HeaderSignature   HTTPHeader = "X-Router-Signature"
	HeaderTimestamp   HTTPHeader = "X-Router-Timestamp"
	HeaderEvent       HTTPHeader = "X-Router-Event"
)

// HTTPValue holds fixed header values and the signature scheme.
type HTTPValue string

const (
	ContentJSON     HTTPValue = "application/json"
	SignaturePrefix HTTPValue = "sha256="
	SignatureJoin   HTTPValue = "."
)

// ResponseLimit caps how much of an error response body is kept for notify_error.
type ResponseLimit int64

const MaxErrorBody ResponseLimit = 512

// JSONPoster posts JSON bodies and turns non-2xx answers into errors.
type JSONPoster struct {
	Client *http.Client
}

// Post sends body to url with extra headers.
func (p JSONPoster) Post(ctx context.Context, url string, body []byte, headers map[HTTPHeader]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(string(HeaderContentType), string(ContentJSON))
	for k, v := range headers {
		req.Header.Set(string(k), v)
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices || resp.StatusCode < http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, int64(MaxErrorBody)))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return nil
}

// WebhookPayload is the JSON body of generic webhooks.
type WebhookPayload struct {
	Type       EventType            `json:"type"`
	RuleID     uuid.UUID            `json:"ruleId"`
	RuleName   string               `json:"ruleName"`
	Kind       alerts.AlertRuleKind `json:"kind"`
	Threshold  float64              `json:"threshold"`
	Window     int                  `json:"windowMinutes"`
	RouteID    uuid.UUID            `json:"routeId"`
	RouteName  string               `json:"routeName"`
	Value      *float64             `json:"value"`
	FiredAt    time.Time            `json:"firedAt"`
	ResolvedAt *time.Time           `json:"resolvedAt,omitempty"`
	Link       string               `json:"link,omitempty"`
}

// WebhookNotifier posts WebhookPayload, signed with HMAC-SHA256 over "<timestamp>.<body>" when a key is set.
type WebhookNotifier struct {
	Poster     JSONPoster
	SigningKey []byte
	Now        func() time.Time
}

func (WebhookNotifier) Type() alerts.ChannelType { return alerts.ChannelWebhook }

// Send implements Notifier.
func (w WebhookNotifier) Send(ctx context.Context, n Notification, d Destination) error {
	body, err := json.Marshal(WebhookPayload{Type: n.Type, RuleID: n.RuleID, RuleName: n.RuleName, Kind: n.Kind, Threshold: n.Threshold,
		Window: int(n.Window), RouteID: n.RouteID, RouteName: n.RouteName, Value: n.Value, FiredAt: n.FiredAt,
		ResolvedAt: n.ResolvedAt, Link: n.Link})
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(w.Now().Unix(), 10)
	headers := map[HTTPHeader]string{HeaderEvent: string(n.Type), HeaderTimestamp: ts}
	if len(w.SigningKey) > 0 {
		headers[HeaderSignature] = WebhookSigner{Key: w.SigningKey}.Sign(ts, body)
	}
	return w.Poster.Post(ctx, d.URL, body, headers)
}

// WebhookSigner computes the X-Router-Signature value receivers verify.
type WebhookSigner struct {
	Key []byte
}

// Sign returns "sha256=<hex hmac(key, ts + "." + body)>".
func (s WebhookSigner) Sign(ts string, body []byte) string {
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte(ts + string(SignatureJoin)))
	mac.Write(body)
	return string(SignaturePrefix) + hex.EncodeToString(mac.Sum(nil))
}

// SlackPayload is an incoming-webhook message.
type SlackPayload struct {
	Text string `json:"text"`
}

// SlackNotifier posts to a Slack incoming webhook.
type SlackNotifier struct {
	Poster JSONPoster
}

func (SlackNotifier) Type() alerts.ChannelType { return alerts.ChannelSlack }

// Send implements Notifier.
func (s SlackNotifier) Send(ctx context.Context, n Notification, d Destination) error {
	m := MessageBuilder{}.Build(n)
	body, err := json.Marshal(SlackPayload{Text: "*" + m.Title + "*" + string(LineBreak) + m.Body})
	if err != nil {
		return err
	}
	return s.Poster.Post(ctx, d.URL, body, nil)
}

// TeamsCard is a Microsoft Teams incoming-webhook MessageCard.
type TeamsCard struct {
	Type       string `json:"@type"`
	Context    string `json:"@context"`
	Summary    string `json:"summary"`
	ThemeColor string `json:"themeColor"`
	Title      string `json:"title"`
	Text       string `json:"text"`
}

// TeamsCardValue holds the fixed MessageCard values.
type TeamsCardValue string

const (
	CardType       TeamsCardValue = "MessageCard"
	CardContext    TeamsCardValue = "http://schema.org/extensions"
	ColorFiring    TeamsCardValue = "D13438"
	ColorResolved  TeamsCardValue = "2EB886"
	TeamsLineBreak TeamsCardValue = "\n\n"
)

// TeamsNotifier posts a MessageCard to a Teams incoming webhook.
type TeamsNotifier struct {
	Poster JSONPoster
}

func (TeamsNotifier) Type() alerts.ChannelType { return alerts.ChannelTeams }

// Send implements Notifier.
func (t TeamsNotifier) Send(ctx context.Context, n Notification, d Destination) error {
	m := MessageBuilder{}.Build(n)
	color := ColorFiring
	if n.Type == EventResolved {
		color = ColorResolved
	}
	text := bytes.ReplaceAll([]byte(m.Body), []byte(LineBreak), []byte(TeamsLineBreak))
	body, err := json.Marshal(TeamsCard{Type: string(CardType), Context: string(CardContext), Summary: m.Title,
		ThemeColor: string(color), Title: m.Title, Text: string(text)})
	if err != nil {
		return err
	}
	return t.Poster.Post(ctx, d.URL, body, nil)
}
