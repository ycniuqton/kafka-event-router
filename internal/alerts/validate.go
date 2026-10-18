package alerts

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// FieldPath is the JSON Pointer of a rule field in problem documents.
type FieldPath string

const (
	PathRouteID   FieldPath = "/routeId"
	PathName      FieldPath = "/name"
	PathKind      FieldPath = "/kind"
	PathThreshold FieldPath = "/threshold"
	PathWindow    FieldPath = "/windowMinutes"
	PathChannels  FieldPath = "/channels"
	PathState     FieldPath = "/state"
)

// ChannelPath returns the pointer of one channel field.
func ChannelPath(i int, field string) string {
	return fmt.Sprintf("%s/%d/%s", PathChannels, i, field)
}

// ChannelField names channel members used in problem paths.
type ChannelField string

const (
	FieldType   ChannelField = "type"
	FieldTarget ChannelField = "target"
	FieldSecret ChannelField = "secret"
)

// URLScheme is the only scheme accepted for webhook channels.
type URLScheme string

const SchemeHTTPS URLScheme = "https"
const SchemeHTTP URLScheme = "http"

// Separator splits email recipient lists.
type Separator string

const RecipientSeparator Separator = ","

// RuleValidator checks a draft against the documented limits; every error is a 422 with a path.
type RuleValidator struct{}

// Validate checks every rule field except channel secrets (checked by ChannelValidator).
func (v RuleValidator) Validate(d RuleDraft) error {
	name := strings.TrimSpace(d.Name)
	switch {
	case name == "" || len(name) > int(MaxNameLength):
		return ErrInvalidName.At(string(PathName))
	case !d.Kind.Valid():
		return ErrInvalidKind.At(string(PathKind))
	case !KindLimits[d.Kind].Contains(d.Threshold):
		return ErrInvalidThreshold.At(string(PathThreshold))
	case !WindowLimits.Contains(float64(d.WindowMinutes)):
		return ErrInvalidWindow.At(string(PathWindow))
	case len(d.Channels) > int(MaxChannels):
		return ErrTooManyChannels.At(string(PathChannels))
	}
	for i, c := range d.Channels {
		if err := (ChannelValidator{}).Validate(i, c); err != nil {
			return err
		}
	}
	return nil
}

// ChannelValidator checks one channel input.
type ChannelValidator struct{}

// Validate checks the type and its target; secrets of URL channels must be absolute http(s) URLs.
func (ChannelValidator) Validate(i int, c ChannelInput) error {
	if !c.Type.Valid() {
		return ErrInvalidChannel.At(ChannelPath(i, string(FieldType)))
	}
	if c.Type == ChannelEmail {
		return EmailList(c.Target).Validate(i)
	}
	if c.Secret == nil {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(*c.Secret))
	if err != nil || u.Host == "" || (u.Scheme != string(SchemeHTTPS) && u.Scheme != string(SchemeHTTP)) {
		return ErrInvalidWebhookURL.At(ChannelPath(i, string(FieldSecret)))
	}
	return nil
}

// EmailList is a comma-separated recipient list.
type EmailList string

// Addresses returns the trimmed, non-empty recipients.
func (l EmailList) Addresses() []string {
	out := []string{}
	for _, part := range strings.Split(string(l), string(RecipientSeparator)) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate requires 1..MaxEmailRecipients parsable addresses.
func (l EmailList) Validate(i int) error {
	addrs := l.Addresses()
	if len(addrs) == 0 || len(addrs) > int(MaxEmailRecipients) {
		return ErrInvalidRecipients.At(ChannelPath(i, string(FieldTarget)))
	}
	for _, a := range addrs {
		if _, err := mail.ParseAddress(a); err != nil {
			return ErrInvalidRecipients.At(ChannelPath(i, string(FieldTarget)))
		}
	}
	return nil
}

// Problems of the alerts feature.
var (
	ErrInvalidName       = problem.NewError(problem.KindValidation, "Give the rule a name of up to 120 characters.")
	ErrInvalidKind       = problem.NewError(problem.KindValidation, "Choose a rule type: lag_above, error_rate_above, throughput_zero or match_rate_change.")
	ErrInvalidThreshold  = problem.NewError(problem.KindValidation, "The threshold is out of range for this rule type (lag ≥ 1, error rate 0.01–100 %, match change 1–100 points, throughput 0).")
	ErrInvalidWindow     = problem.NewError(problem.KindValidation, "The window must be between 1 and 1440 minutes.")
	ErrTooManyChannels   = problem.NewError(problem.KindValidation, "A rule can notify at most 10 channels.")
	ErrInvalidChannel    = problem.NewError(problem.KindValidation, "Choose a channel type: email, slack, teams or webhook.")
	ErrInvalidRecipients = problem.NewError(problem.KindValidation, "Enter 1 to 20 email addresses separated by commas.")
	ErrInvalidWebhookURL = problem.NewError(problem.KindValidation, "Enter the full https:// URL of the incoming webhook.")
	ErrMissingSecret     = problem.NewError(problem.KindValidation, "Enter the webhook URL for this channel.")
	ErrRouteArchived     = problem.NewError(problem.KindValidation, "This route is archived. Unarchive it before adding alert rules.")
	ErrRouteUnknown      = problem.NewError(problem.KindValidation, "This route does not exist. Pick an existing route.")
	ErrRuleNotFound      = problem.NewError(problem.KindNotFound, "This alert rule does not exist. It may have been deleted.")
	ErrAlertNotFound     = problem.NewError(problem.KindNotFound, "This alert does not exist.")
	ErrWriteForbidden    = problem.NewError(problem.KindForbidden, "You need the editor role on the route's team to change its alert rules.")
	ErrInvalidState      = problem.NewError(problem.KindMalformedBody, "state must be firing or resolved.")
)
