package operators

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// unitDurations maps duration units to their length.
var unitDurations = map[config.DurationUnit]time.Duration{
	config.DurationUnitSeconds: time.Second,
	config.DurationUnitMinutes: time.Minute,
	config.DurationUnitHours:   time.Hour,
	config.DurationUnitDays:    24 * time.Hour,
}

// WithinLast matches times in [now - amount*unit, now] using the frozen event clock.
type WithinLast struct{ base }

func (WithinLast) Op() config.OpName           { return config.OpNameWithinLast }
func (WithinLast) Accepts() []filter.FieldType { return dateTypes }
func (WithinLast) Operand() filter.OperandKind { return filter.OperandDuration }
func (WithinLast) Prepare(o filter.Operand) (filter.Prepared, error) {
	if o.Amount == nil || o.Unit == nil || *o.Amount <= 0 {
		return nil, filter.NewOperandError(config.IssueOperandInvalid, config.TokenAmount, "within_last requires a positive amount and a unit")
	}
	unit, ok := unitDurations[*o.Unit]
	if !ok {
		return nil, filter.NewOperandError(config.IssueOperandInvalid, config.TokenUnit, "unknown unit %q", *o.Unit)
	}
	return withinMatch{window: time.Duration(*o.Amount * float64(unit))}, nil
}

type withinMatch struct {
	window time.Duration
}

func (p withinMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	t, ok := in.Coercer.Time(in.Left)
	if !ok {
		return false, &filter.CoercionWarning{Code: filter.WarnNotADate}
	}
	return !t.Before(in.Now.Add(-p.window)) && !t.After(in.Now), nil
}
