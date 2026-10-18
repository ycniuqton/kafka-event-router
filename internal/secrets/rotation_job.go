package secrets

import (
	"context"
	"log/slog"
	"time"
)

// Leadership elects one API instance (route-runtime's scheduler.PgLeaderElector with its own lock key).
type Leadership interface {
	Campaign(ctx context.Context) (lost <-chan struct{}, err error)
}

// RotationLockKey is the advisory lock of the rotation job (7001 migrations, 7008 scheduler, 7110 dlq janitor).
type RotationLockKey int64

const RotationJobLockKey RotationLockKey = 7120

// RotationTiming is the delay before the first run after start.
type RotationTiming time.Duration

const RotationFirstDelay RotationTiming = RotationTiming(2 * time.Minute)

// RotationLog names rotation job log messages and attributes.
type RotationLog string

const (
	LogRotationDone    RotationLog = "secrets rotation: envelopes re-wrapped"
	LogRotationFailed  RotationLog = "secrets rotation: run stopped; retried next interval"
	LogAttrRotated     RotationLog = "rotated"
	LogAttrRaced       RotationLog = "raced"
	LogAttrToKeyID     RotationLog = "to_key_id"
	LogAttrFailedRef   RotationLog = "failed_ref"
	LogAttrRotationErr RotationLog = "error"
)

// RotationJob runs the KeyRotator on the leader only, after RotationFirstDelay and then every Interval.
type RotationJob struct {
	Rotator    KeyRotator
	Leader     Leadership
	Interval   time.Duration
	FirstDelay time.Duration
	Log        *slog.Logger
}

// Run loops until ctx ends.
func (j RotationJob) Run(ctx context.Context) {
	timer := time.NewTimer(j.FirstDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			j.Once(ctx)
			timer.Reset(j.Interval)
		}
	}
}

// Once rotates when this instance wins leadership and releases the lock afterwards.
func (j RotationJob) Once(ctx context.Context) (RotationReport, bool) {
	lead, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := j.Leader.Campaign(lead); err != nil {
		return RotationReport{}, false
	}
	report, err := j.Rotator.Run(lead)
	if err != nil {
		j.Log.Error(string(LogRotationFailed), string(LogAttrRotated), report.Rotated,
			string(LogAttrFailedRef), report.Failed, string(LogAttrRotationErr), err)
		return report, true
	}
	if report.Rotated > 0 || report.Raced > 0 {
		j.Log.Info(string(LogRotationDone), string(LogAttrRotated), report.Rotated, string(LogAttrRaced), report.Raced,
			string(LogAttrToKeyID), report.ToKeyID)
	}
	return report, true
}
