package notify

import (
	"context"
	"errors"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/alerts"
)

// SMTPSettings come from ROUTER_SMTP_* (host, port, user, password, from).
type SMTPSettings struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Configured reports whether email can be sent.
func (s SMTPSettings) Configured() bool {
	return s.Host != "" && s.Port > 0 && s.From != ""
}

// Addr returns host:port.
func (s SMTPSettings) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// MailHeader names the headers of alert emails.
type MailHeader string

const (
	MailFrom        MailHeader = "From: "
	MailTo          MailHeader = "To: "
	MailSubject     MailHeader = "Subject: "
	MailContentType MailHeader = "Content-Type: text/plain; charset=UTF-8"
	MailCRLF        MailHeader = "\r\n"
	MailListJoin    MailHeader = ", "
)

// ErrNoRecipients is returned for an email channel without recipients.
var ErrNoRecipients = errors.New("no email recipients")

// ErrSMTPNotConfigured is returned when the SMTP settings are incomplete.
var ErrSMTPNotConfigured = errors.New("smtp not configured (set ROUTER_SMTP_HOST, ROUTER_SMTP_PORT, ROUTER_SMTP_FROM)")

// MailSender delivers one message (net/smtp.SendMail in production).
type MailSender func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// EmailNotifier sends plain-text alert emails over SMTP.
type EmailNotifier struct {
	SMTP   SMTPSettings
	Mailer MailSender
}

func (EmailNotifier) Type() alerts.ChannelType { return alerts.ChannelEmail }

// Send implements Notifier.
func (e EmailNotifier) Send(_ context.Context, n Notification, d Destination) error {
	if !e.SMTP.Configured() {
		return ErrSMTPNotConfigured
	}
	if len(d.Recipients) == 0 {
		return ErrNoRecipients
	}
	var auth smtp.Auth
	if e.SMTP.User != "" {
		auth = smtp.PlainAuth("", e.SMTP.User, e.SMTP.Password, e.SMTP.Host)
	}
	return e.Mailer(e.SMTP.Addr(), auth, e.SMTP.From, d.Recipients, MailComposer{}.Compose(e.SMTP.From, d.Recipients, MessageBuilder{}.Build(n)))
}

// MailComposer builds an RFC 5322 message.
type MailComposer struct{}

// Compose returns headers + body with CRLF line endings.
func (MailComposer) Compose(from string, to []string, m Message) []byte {
	crlf := string(MailCRLF)
	var b strings.Builder
	b.WriteString(string(MailFrom) + from + crlf)
	b.WriteString(string(MailTo) + strings.Join(to, string(MailListJoin)) + crlf)
	b.WriteString(string(MailSubject) + m.Title + crlf)
	b.WriteString(string(MailContentType) + crlf + crlf)
	b.WriteString(strings.ReplaceAll(m.Body, string(LineBreak), crlf) + crlf)
	return []byte(b.String())
}
