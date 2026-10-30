package config

import "time"

// RetryDefault holds the defaults of errorPolicy.retry members left out of a route document.
type RetryDefault int

const (
	DefaultRetryAttempts    RetryDefault = 5
	DefaultRetryBaseDelayMs RetryDefault = 1000
	DefaultRetryMaxDelayMs  RetryDefault = 300000
)

// ResolvedRetryPolicy is errorPolicy.retry with every default applied.
type ResolvedRetryPolicy struct {
	Enabled     bool
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// RetryPolicyOf resolves the retry policy of an error policy (absent policy or retry = disabled).
func RetryPolicyOf(ep *ErrorPolicy) ResolvedRetryPolicy {
	out := ResolvedRetryPolicy{MaxAttempts: int(DefaultRetryAttempts),
		BaseDelay: RetryMillis{}.Of(nil, DefaultRetryBaseDelayMs), MaxDelay: RetryMillis{}.Of(nil, DefaultRetryMaxDelayMs)}
	if ep == nil || ep.Retry == nil {
		return out
	}
	r := ep.Retry
	out.Enabled = r.Enabled
	out.BaseDelay = RetryMillis{}.Of(r.BaseDelayMs, DefaultRetryBaseDelayMs)
	out.MaxDelay = RetryMillis{}.Of(r.MaxDelayMs, DefaultRetryMaxDelayMs)
	if r.MaxAttempts != nil {
		out.MaxAttempts = *r.MaxAttempts
	}
	return out
}

// RetryMillis converts optional millisecond members.
type RetryMillis struct{}

// Of returns v milliseconds, or the default when v is absent.
func (RetryMillis) Of(v *int, def RetryDefault) time.Duration {
	if v == nil {
		return time.Duration(def) * time.Millisecond
	}
	return time.Duration(*v) * time.Millisecond
}
