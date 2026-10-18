package quickjs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/sys"

	"github.com/eventrouter/router/pkg/sandbox"
)

var (
	errGuestOutOfMemory = errors.New("guest allocation failed")
	errGuestOutOfBounds = errors.New("guest memory access out of bounds")
	errGuestNotString   = errors.New("guest value is not convertible to a string")
	errInstanceClosed   = errors.New("sandbox instance is closed")
	errBadBridgeCall    = errors.New("malformed host bridge call")
	errGuestBoot        = errors.New("quickjs guest failed to boot")
)

// tooLargeError reports a guest string above the read limit.
type tooLargeError struct {
	size int64
}

func (e *tooLargeError) Error() string {
	return fmt.Sprintf("guest string of %d bytes exceeds the limit", e.size)
}

// joinNil returns the first non-nil error.
func joinNil(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// trapSignature is text wazero puts in errors of a given trap.
type trapSignature string

const trapOutOfBounds trapSignature = "out of bounds memory access"

// trapClassifier maps wazero call failures to sandbox errors.
type trapClassifier struct {
	classifier sandbox.ErrorClassifier
	limits     sandbox.Limits
}

// classify decides timeout, stack overflow, size or internal for err raised while callCtx ran.
func (t trapClassifier) classify(callCtx context.Context, err error) *sandbox.Error {
	var tl *tooLargeError
	if errors.As(err, &tl) {
		return t.classifier.New(sandbox.ErrorOutputTooLarge, sandbox.MsgOutputTooLarge.Format(tl.size, t.limits.MaxOutputBytes))
	}
	if t.isDeadline(callCtx, err) {
		return t.classifier.New(sandbox.ErrorTimeout, sandbox.MsgTimeout.Format(t.limits.Timeout.Milliseconds()))
	}
	if strings.Contains(err.Error(), string(trapOutOfBounds)) {
		return t.classifier.New(sandbox.ErrorStackOverflow, string(sandbox.MsgStackOverflow))
	}
	return t.classifier.New(sandbox.ErrorInternal, sandbox.MsgInstanceTrapped.Format(err))
}

func (trapClassifier) isDeadline(callCtx context.Context, err error) bool {
	var exit *sys.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == sys.ExitCodeDeadlineExceeded {
		return true
	}
	return errors.Is(callCtx.Err(), context.DeadlineExceeded)
}
