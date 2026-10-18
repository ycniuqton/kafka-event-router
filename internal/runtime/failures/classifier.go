// Package failures classifies runtime errors, maps categories to dispositions, applies the route's
// errorPolicy (dlq / skip / pause) and writes dead letters through the dead-letters record contract.
package failures

import (
	"context"
	"errors"
	"net"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// ErrorClassifier maps typed errors from the engine, the sandbox (via engine) and franz-go to categories.
type ErrorClassifier interface {
	Classify(err error) domain.ErrorCategory
	ClassifySource(err error) domain.ErrorCategory
}

// engineCategories maps engine error types to runtime categories.
var engineCategories = map[engine.ErrorType]domain.ErrorCategory{
	engine.ErrDecode:          domain.CategoryDecode,
	engine.ErrDecodeTransient: domain.CategorySourceTransient,
	engine.ErrStep:            domain.CategoryStep,
	engine.ErrStepTimeout:     domain.CategoryStepTimeout,
	engine.ErrOutputInvalid:   domain.CategoryOutputInvalid,
	engine.ErrLimit:           domain.CategoryStep,
}

// transientClientErrors are franz-go client errors that never mean data is undeliverable.
var transientClientErrors = []error{kgo.ErrRecordTimeout, kgo.ErrRecordRetries, kgo.ErrClientClosed, kgo.ErrMaxBuffered, context.Canceled, context.DeadlineExceeded}

// permanentDestinationCodes are broker codes that never heal by retrying a produce (missing topic, ACLs, size).
var permanentDestinationCodes = []*kerr.Error{kerr.UnknownTopicOrPartition, kerr.TopicAuthorizationFailed, kerr.ClusterAuthorizationFailed,
	kerr.MessageTooLarge, kerr.RecordListTooLarge, kerr.InvalidTopicException, kerr.SaslAuthenticationFailed}

// fatalSourceCodes are broker codes after which the source cannot be read without operator action.
var fatalSourceCodes = []*kerr.Error{kerr.TopicAuthorizationFailed, kerr.GroupAuthorizationFailed, kerr.ClusterAuthorizationFailed,
	kerr.SaslAuthenticationFailed, kerr.InvalidGroupID}

// DefaultClassifier is the fixed classification of the MVP.
type DefaultClassifier struct{}

// Classify classifies processing and destination (produce) failures.
func (DefaultClassifier) Classify(err error) domain.ErrorCategory {
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		return EngineErrors{}.Category(pe)
	}
	return KafkaErrors{}.Destination(err)
}

// EngineErrors classifies typed engine failures.
type EngineErrors struct{}

// Category maps the engine type; an output_invalid caused by an unreachable dependency (schema registry
// outage, timeout) is destination_transient: the record is fine, so it is retried, never dead-lettered.
func (EngineErrors) Category(pe *engine.ProcessError) domain.ErrorCategory {
	c, ok := engineCategories[pe.Type]
	if !ok {
		return domain.CategoryStep
	}
	if c == domain.CategoryOutputInvalid && (TransientCauses{}).Match(pe.Cause) {
		return domain.CategoryDestinationTransient
	}
	return c
}

// temporary is the convention of errors that know they may heal on retry.
type temporary interface {
	Temporary() bool
}

// TransientCauses recognises causes that say nothing about the record: codec.ErrTransient (registry outage),
// timeouts, network and temporary errors.
type TransientCauses struct{}

// Match reports whether err is such a cause.
func (TransientCauses) Match(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, codec.ErrTransient) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var tmp temporary
	return errors.As(err, &tmp) && tmp.Temporary()
}

// ClassifySource classifies fetch and group errors of the source consumer.
func (DefaultClassifier) ClassifySource(err error) domain.ErrorCategory {
	for _, code := range fatalSourceCodes {
		if errors.Is(err, code) {
			return domain.CategorySourceFatal
		}
	}
	return domain.CategorySourceTransient
}

// KafkaErrors classifies franz-go and broker errors.
type KafkaErrors struct{}

// Destination returns destination_transient for retriable, network and client errors, else permanent.
func (KafkaErrors) Destination(err error) domain.ErrorCategory {
	for _, code := range permanentDestinationCodes {
		if errors.Is(err, code) {
			return domain.CategoryDestinationPermanent
		}
	}
	for _, t := range transientClientErrors {
		if errors.Is(err, t) {
			return domain.CategoryDestinationTransient
		}
	}
	var ke *kerr.Error
	if errors.As(err, &ke) {
		if ke.Retriable {
			return domain.CategoryDestinationTransient
		}
		return domain.CategoryDestinationPermanent
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return domain.CategoryDestinationTransient
	}
	return domain.CategoryDestinationPermanent
}
