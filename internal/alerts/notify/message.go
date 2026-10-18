package notify

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/alerts"
)

// Wording holds the user-facing text of notifications.
type Wording string

const (
	TitleFired       Wording = "[FIRING] %s · %s"
	TitleResolved    Wording = "[RESOLVED] %s · %s"
	TitleTest        Wording = "[TEST] %s · %s"
	ConditionLag     Wording = "Consumer lag stayed above %s for %d min"
	ConditionErrors  Wording = "Error rate above %s %% over %d min"
	ConditionZero    Wording = "No events sent for %d min"
	ConditionMatch   Wording = "Match rate changed by more than %s points vs. the previous %d min"
	ValueLine        Wording = "Current value: %s"
	NoValue          Wording = "no data"
	ResolvedLine     Wording = "The condition is back to normal."
	TestLine         Wording = "This is a test notification from Event Router."
	OpenLine         Wording = "Open the route: %s"
	LineBreak        Wording = "\n"
	ValuePrecisionFm Wording = "g"
)

// Message is a rendered notification: one-line title plus a plain-text body.
type Message struct {
	Title string
	Body  string
}

// MessageBuilder renders a Notification for humans (email, Slack, Teams).
type MessageBuilder struct{}

// Build returns the title and body of n.
func (b MessageBuilder) Build(n Notification) Message {
	title := TitleFired
	switch n.Type {
	case EventResolved:
		title = TitleResolved
	case EventTest:
		title = TitleTest
	}
	lines := []string{b.Condition(n), fmt.Sprintf(string(ValueLine), b.value(n.Value))}
	if n.Type == EventResolved {
		lines = append(lines, string(ResolvedLine))
	}
	if n.Type == EventTest {
		lines = append(lines, string(TestLine))
	}
	if n.Link != "" {
		lines = append(lines, fmt.Sprintf(string(OpenLine), n.Link))
	}
	return Message{Title: fmt.Sprintf(string(title), n.RuleName, n.RouteName), Body: strings.Join(lines, string(LineBreak))}
}

// Condition describes the rule in one sentence.
func (b MessageBuilder) Condition(n Notification) string {
	w := int(n.Window)
	switch n.Kind {
	case alerts.KindLagAbove:
		return fmt.Sprintf(string(ConditionLag), b.number(n.Threshold), w)
	case alerts.KindErrorRateAbove:
		return fmt.Sprintf(string(ConditionErrors), b.number(n.Threshold), w)
	case alerts.KindThroughputZero:
		return fmt.Sprintf(string(ConditionZero), w)
	default:
		return fmt.Sprintf(string(ConditionMatch), b.number(n.Threshold), w)
	}
}

func (b MessageBuilder) value(v *float64) string {
	if v == nil {
		return string(NoValue)
	}
	return b.number(*v)
}

func (MessageBuilder) number(v float64) string {
	return strconv.FormatFloat(v, ValuePrecisionFm[0], -1, 64)
}
