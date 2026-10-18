package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/twmb/franz-go/pkg/kerr"
	"golang.org/x/oauth2"
)

// FailureReason is the plain-language cause of a cluster error.
type FailureReason string

const (
	ReasonUnreachable   FailureReason = "unreachable"
	ReasonDNSFailure    FailureReason = "dns_failure"
	ReasonTLSHandshake  FailureReason = "tls_handshake"
	ReasonTLSUntrusted  FailureReason = "tls_untrusted"
	ReasonAuthFailed    FailureReason = "auth_failed"
	ReasonSASLMismatch  FailureReason = "sasl_mechanism_mismatch"
	ReasonNotAuthorized FailureReason = "not_authorized"
	ReasonTopicNotFound FailureReason = "topic_not_found"
	ReasonTopicExists   FailureReason = "topic_exists"
	ReasonInvalidConfig FailureReason = "invalid_config"
	ReasonTimeout       FailureReason = "timeout"
	ReasonUnknown       FailureReason = "unknown"
)

// ClusterError carries a classified cluster failure.
type ClusterError struct {
	Reason FailureReason
	Err    error
}

func (e *ClusterError) Error() string { return "kafka: " + string(e.Reason) + ": " + e.Err.Error() }
func (e *ClusterError) Unwrap() error { return e.Err }

// ErrorClassifier recognises one family of errors.
type ErrorClassifier interface {
	Classify(err error) (FailureReason, bool)
}

// NetErrorClassifier recognises DNS, refused/unreachable and timeout errors.
type NetErrorClassifier struct{}

// Classify implements ErrorClassifier.
func (NetErrorClassifier) Classify(err error) (FailureReason, bool) {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ReasonDNSFailure, true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return ReasonTimeout, true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return ReasonUnreachable, true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ReasonUnreachable, true
	}
	return "", false
}

// TLSMarker is the substring franz-go/crypto use in handshake failures.
type TLSMarker string

const TLSErrorMarker TLSMarker = "tls: "

// TLSErrorClassifier recognises certificate trust and handshake failures.
type TLSErrorClassifier struct{}

// Classify implements ErrorClassifier.
func (TLSErrorClassifier) Classify(err error) (FailureReason, bool) {
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verifyErr *tls.CertificateVerificationError
	if errors.As(err, &unknownAuth) || errors.As(err, &hostErr) || errors.As(err, &invalid) || errors.As(err, &verifyErr) {
		return ReasonTLSUntrusted, true
	}
	var header tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &header) || errors.As(err, &alert) || strings.Contains(err.Error(), string(TLSErrorMarker)) {
		return ReasonTLSHandshake, true
	}
	return "", false
}

// OAuthErrorClassifier recognises token endpoint rejections (bad client id / secret / scope).
type OAuthErrorClassifier struct{}

// Classify implements ErrorClassifier; 4xx answers are credential problems, others fall through.
func (OAuthErrorClassifier) Classify(err error) (FailureReason, bool) {
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) || retrieve.Response == nil {
		return "", false
	}
	if retrieve.Response.StatusCode >= http.StatusBadRequest && retrieve.Response.StatusCode < http.StatusInternalServerError {
		return ReasonAuthFailed, true
	}
	return ReasonUnreachable, true
}

// KafkaCodeTable maps broker error codes to reasons.
type KafkaCodeTable map[*kerr.Error]FailureReason

// SASLErrorClassifier recognises SASL authentication and mechanism errors.
type SASLErrorClassifier struct {
	codes KafkaCodeTable
}

// NewSASLErrorClassifier returns the SASL classifier with its code table.
func NewSASLErrorClassifier() SASLErrorClassifier {
	return SASLErrorClassifier{codes: KafkaCodeTable{
		kerr.SaslAuthenticationFailed: ReasonAuthFailed,
		kerr.UnsupportedSaslMechanism: ReasonSASLMismatch,
		kerr.IllegalSaslState:         ReasonSASLMismatch,
	}}
}

// Classify implements ErrorClassifier.
func (c SASLErrorClassifier) Classify(err error) (FailureReason, bool) { return c.codes.Match(err) }

// KafkaErrorClassifier recognises authorization, topic and timeout broker codes.
type KafkaErrorClassifier struct {
	codes KafkaCodeTable
}

// NewKafkaErrorClassifier returns the broker-code classifier.
func NewKafkaErrorClassifier() KafkaErrorClassifier {
	return KafkaErrorClassifier{codes: KafkaCodeTable{
		kerr.TopicAuthorizationFailed:   ReasonNotAuthorized,
		kerr.ClusterAuthorizationFailed: ReasonNotAuthorized,
		kerr.GroupAuthorizationFailed:   ReasonNotAuthorized,
		kerr.UnknownTopicOrPartition:    ReasonTopicNotFound,
		kerr.TopicAlreadyExists:         ReasonTopicExists,
		kerr.InvalidTopicException:      ReasonInvalidConfig,
		kerr.InvalidPartitions:          ReasonInvalidConfig,
		kerr.InvalidReplicationFactor:   ReasonInvalidConfig,
		kerr.InvalidConfig:              ReasonInvalidConfig,
		kerr.RequestTimedOut:            ReasonTimeout,
	}}
}

// Classify implements ErrorClassifier.
func (c KafkaErrorClassifier) Classify(err error) (FailureReason, bool) { return c.codes.Match(err) }

// Match returns the reason of the first code err matches.
func (t KafkaCodeTable) Match(err error) (FailureReason, bool) {
	for code, reason := range t {
		if errors.Is(err, code) {
			return reason, true
		}
	}
	return "", false
}

// FailureClassifier is the Manager running the classifier chain; it falls back to unknown.
type FailureClassifier struct {
	chain []ErrorClassifier
}

// NewFailureClassifier builds a manager over an ordered chain.
func NewFailureClassifier(chain ...ErrorClassifier) *FailureClassifier {
	return &FailureClassifier{chain: chain}
}

// DefaultFailureClassifier orders classifiers from most to least specific.
func DefaultFailureClassifier() *FailureClassifier {
	return NewFailureClassifier(NewSASLErrorClassifier(), OAuthErrorClassifier{}, NewKafkaErrorClassifier(), TLSErrorClassifier{}, NetErrorClassifier{})
}

// Classify returns the reason of err (ClusterError reasons are kept).
func (f *FailureClassifier) Classify(err error) FailureReason {
	var ce *ClusterError
	if errors.As(err, &ce) {
		return ce.Reason
	}
	for _, c := range f.chain {
		if reason, ok := c.Classify(err); ok {
			return reason
		}
	}
	return ReasonUnknown
}

// Wrap returns err as a ClusterError carrying its reason.
func (f *FailureClassifier) Wrap(err error) *ClusterError {
	var ce *ClusterError
	if errors.As(err, &ce) {
		return ce
	}
	return &ClusterError{Reason: f.Classify(err), Err: err}
}
