package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ExportSinkKind selects the continuous SIEM export sink (ROUTER_AUDIT_EXPORT_SINK).
type ExportSinkKind string

const (
	ExportSinkNone    ExportSinkKind = "none"
	ExportSinkWebhook ExportSinkKind = "webhook"
	ExportSinkSyslog  ExportSinkKind = "syslog"
)

// ExportSink is the base of the extensible SIEM sink domain: it delivers one batch or fails as a whole.
type ExportSink interface {
	Kind() ExportSinkKind
	Send(ctx context.Context, batch []LogEntry) error
}

// ExportSinkDeps are the inputs every sink factory receives.
type ExportSinkDeps struct {
	Target  string
	Timeout time.Duration
	Client  *http.Client
	Host    string
}

// ExportSinkFactory builds one sink implementation.
type ExportSinkFactory func(ExportSinkDeps) (ExportSink, error)

// ErrUnknownExportSink is returned for a ROUTER_AUDIT_EXPORT_SINK without a registered factory.
var ErrUnknownExportSink = errors.New("audit: no export sink registered for this kind")

// ErrExportSinkTarget is returned when the configured target does not suit the sink.
var ErrExportSinkTarget = errors.New("audit: export sink target is not valid")

// ExportSinkRegistry maps sink kinds to factories (config-selected, no switch on concrete types).
type ExportSinkRegistry struct {
	factories map[ExportSinkKind]ExportSinkFactory
}

// NewExportSinkRegistry returns the registry with the none, webhook and syslog sinks.
func NewExportSinkRegistry() *ExportSinkRegistry {
	r := &ExportSinkRegistry{factories: map[ExportSinkKind]ExportSinkFactory{}}
	r.Register(ExportSinkNone, func(ExportSinkDeps) (ExportSink, error) { return NoopExportSink{}, nil })
	r.Register(ExportSinkWebhook, NewWebhookExportSink)
	r.Register(ExportSinkSyslog, NewSyslogExportSink)
	return r
}

// Register adds or replaces the factory of kind.
func (r *ExportSinkRegistry) Register(kind ExportSinkKind, f ExportSinkFactory) {
	r.factories[kind] = f
}

// Build instantiates the configured sink.
func (r *ExportSinkRegistry) Build(kind ExportSinkKind, d ExportSinkDeps) (ExportSink, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownExportSink, kind)
	}
	return f(d)
}

// NoopExportSink disables continuous export (the job is not started for it).
type NoopExportSink struct{}

func (NoopExportSink) Kind() ExportSinkKind                   { return ExportSinkNone }
func (NoopExportSink) Send(context.Context, []LogEntry) error { return nil }

// WebhookExportSink POSTs each batch as NDJSON to an HTTP(S) collector; any non-2xx answer fails the batch.
type WebhookExportSink struct {
	target string
	client *http.Client
}

// NewWebhookExportSink validates the target URL.
func NewWebhookExportSink(d ExportSinkDeps) (ExportSink, error) {
	u, err := url.Parse(d.Target)
	if err != nil || (u.Scheme != string(SchemeHTTP) && u.Scheme != string(SchemeHTTPS)) || u.Host == "" {
		return nil, fmt.Errorf("%w: webhook needs an http(s) URL", ErrExportSinkTarget)
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: d.Timeout}
	}
	return &WebhookExportSink{target: d.Target, client: client}, nil
}

func (s *WebhookExportSink) Kind() ExportSinkKind { return ExportSinkWebhook }

// Send implements ExportSink.
func (s *WebhookExportSink) Send(ctx context.Context, batch []LogEntry) error {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	for _, e := range batch {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.target, &body)
	if err != nil {
		return err
	}
	req.Header.Set(string(ExportHeaderContentType), string(MediaJSONL))
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < int(StatusSuccessMin) || resp.StatusCode >= int(StatusSuccessMax) {
		return fmt.Errorf("audit: webhook answered %d", resp.StatusCode)
	}
	return nil
}

// TargetScheme names the URL schemes sink targets use.
type TargetScheme string

const (
	SchemeHTTP  TargetScheme = "http"
	SchemeHTTPS TargetScheme = "https"
	SchemeUDP   TargetScheme = "udp"
	SchemeTCP   TargetScheme = "tcp"
)

// ExportSinkHeader names HTTP headers sinks send.
type ExportSinkHeader string

const ExportHeaderContentType ExportSinkHeader = "Content-Type"

// SyslogFormat holds the RFC 5424 framing of one audit message.
type SyslogFormat string

const (
	// SyslogLine is "<PRI>1 TIMESTAMP HOST APP - MSGID - MSG"; PRI 110 = facility log audit (13) * 8 + info (6).
	SyslogLine        SyslogFormat = "<110>1 %s %s event-router - %s - %s"
	SyslogTCPFrame    SyslogFormat = "%d %s"
	SyslogUnknownHost SyslogFormat = "-"
)

// SyslogExportSink sends one RFC 5424 message per entry (JSON body) over UDP or TCP (octet-counted framing).
type SyslogExportSink struct {
	scheme  TargetScheme
	addr    string
	host    string
	timeout time.Duration
}

// NewSyslogExportSink validates a udp://host:port or tcp://host:port target.
func NewSyslogExportSink(d ExportSinkDeps) (ExportSink, error) {
	u, err := url.Parse(d.Target)
	if err != nil || (u.Scheme != string(SchemeUDP) && u.Scheme != string(SchemeTCP)) || u.Port() == "" {
		return nil, fmt.Errorf("%w: syslog needs udp://host:port or tcp://host:port", ErrExportSinkTarget)
	}
	host := d.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	if host == "" {
		host = string(SyslogUnknownHost)
	}
	return &SyslogExportSink{scheme: TargetScheme(u.Scheme), addr: u.Host, host: host, timeout: d.Timeout}, nil
}

func (s *SyslogExportSink) Kind() ExportSinkKind { return ExportSinkSyslog }

// Send implements ExportSink over one connection per batch.
func (s *SyslogExportSink) Send(ctx context.Context, batch []LogEntry) error {
	dialer := net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, string(s.scheme), s.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if s.timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(s.timeout))
	}
	for _, e := range batch {
		msg, err := s.Message(e)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(conn, s.frame(msg)); err != nil {
			return err
		}
	}
	return nil
}

// Message renders e as one RFC 5424 line (MSGID = action, MSG = the JSON entry).
func (s *SyslogExportSink) Message(e LogEntry) (string, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(string(SyslogLine), e.At.UTC().Format(time.RFC3339Nano), s.host, e.Action, body), nil
}

// frame applies RFC 6587 octet counting on TCP; UDP sends one datagram per message.
func (s *SyslogExportSink) frame(msg string) string {
	if s.scheme == SchemeTCP {
		return fmt.Sprintf(string(SyslogTCPFrame), len(msg), msg)
	}
	return msg
}
