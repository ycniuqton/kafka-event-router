package recipes

import (
	"errors"
	"fmt"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/step"
)

// ErrNotDate is the cause when format_date cannot parse its input.
var ErrNotDate = errors.New("value is not an RFC 3339 date or epoch milliseconds")

// FormatDate renders a date (RFC 3339 text or epoch ms) in a layout and IANA time zone.
type FormatDate struct{}

func (FormatDate) Kind() config.StepKind { return config.StepKindFormatDate }
func (FormatDate) Group() step.Group     { return step.GroupTypes }

func (FormatDate) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	if raw.Layout == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenLayout, "format_date requires layout")
		return nil, false
	}
	m := dateMapper{layout: ctxhelpers.DateLayout(*raw.Layout)}
	if raw.Timezone != nil {
		m.tz = *raw.Timezone
	}
	_, err := c.Scope.Helpers.Date.Format(c.Scope.Helpers.Clock.Now(), m.layout, m.tz)
	if err != nil {
		c.Fail(config.IssueOperandInvalid, config.TokenTimezone, "%v", err)
	}
	return FieldTransformBuilder{}.Build(c, raw, m, err == nil)
}

type dateMapper struct {
	layout ctxhelpers.DateLayout
	tz     string
}

func (m dateMapper) Map(sc *step.StepContext, v any) (any, error) {
	t, ok := (filter.Coercer{}).Time(v)
	if !ok {
		return nil, fmt.Errorf("%w: %v", ErrNotDate, v)
	}
	return sc.Helpers.Date.Format(t, m.layout, m.tz)
}
