package ctxhelpers

import (
	"fmt"
	"time"
)

// Clock supplies processing time; the engine reads it once per event and freezes it.
type Clock interface {
	Now() time.Time
}

// SystemClock is wall-clock processing time (production).
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock always returns T (preview, tests, routerctl --now).
type FixedClock struct {
	T time.Time
}

func (c FixedClock) Now() time.Time { return c.T }

// DateLayout names a supported output layout of formatDate.
type DateLayout string

const (
	LayoutRFC3339   DateLayout = "rfc3339"
	LayoutRFC3339Ms DateLayout = "rfc3339_ms"
	LayoutDate      DateLayout = "date"
	LayoutDateTime  DateLayout = "datetime"
)

// DateFormatter formats a time in a layout and IANA time zone.
type DateFormatter interface {
	Format(t time.Time, layout DateLayout, tz string) (string, error)
}

// LayoutDateFormatter maps DateLayout names to Go layouts.
type LayoutDateFormatter struct{}

var dateLayouts = map[DateLayout]string{
	LayoutRFC3339:   time.RFC3339,
	LayoutRFC3339Ms: "2006-01-02T15:04:05.000Z07:00",
	LayoutDate:      time.DateOnly,
	LayoutDateTime:  time.DateTime,
}

// Format renders t; an empty tz means UTC.
func (LayoutDateFormatter) Format(t time.Time, layout DateLayout, tz string) (string, error) {
	goLayout, ok := dateLayouts[layout]
	if !ok {
		return "", fmt.Errorf("unknown date layout %q", layout)
	}
	loc := time.UTC
	if tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil {
			return "", err
		}
		loc = l
	}
	return t.In(loc).Format(goLayout), nil
}

// Layouts lists supported layouts (engine catalogue).
func (LayoutDateFormatter) Layouts() []DateLayout {
	return []DateLayout{LayoutRFC3339, LayoutRFC3339Ms, LayoutDate, LayoutDateTime}
}
