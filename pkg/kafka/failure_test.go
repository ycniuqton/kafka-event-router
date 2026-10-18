package kafka

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
	"golang.org/x/oauth2"
)

func TestFailureClassifier(t *testing.T) {
	fc := DefaultFailureClassifier()
	cases := []struct {
		name string
		err  error
		want FailureReason
	}{
		{"dns", &net.DNSError{Err: "no such host", Name: "kafka-1"}, ReasonDNSFailure},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, ReasonUnreachable},
		{"op error", &net.OpError{Op: "dial", Err: errors.New("boom")}, ReasonUnreachable},
		{"deadline", fmt.Errorf("wrap: %w", context.DeadlineExceeded), ReasonTimeout},
		{"untrusted", fmt.Errorf("x: %w", x509.UnknownAuthorityError{}), ReasonTLSUntrusted},
		{"handshake", errors.New("remote error: tls: bad certificate"), ReasonTLSHandshake},
		{"sasl auth", fmt.Errorf("x: %w", kerr.SaslAuthenticationFailed), ReasonAuthFailed},
		{"sasl mech", kerr.UnsupportedSaslMechanism, ReasonSASLMismatch},
		{"topic authz", kerr.TopicAuthorizationFailed, ReasonNotAuthorized},
		{"unknown topic", kerr.UnknownTopicOrPartition, ReasonTopicNotFound},
		{"exists", kerr.TopicAlreadyExists, ReasonTopicExists},
		{"cluster error kept", &ClusterError{Reason: ReasonTimeout, Err: errors.New("x")}, ReasonTimeout},
		{"oauth rejected", fmt.Errorf("x: %w", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized}}), ReasonAuthFailed},
		{"oauth idp down", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusBadGateway}}, ReasonUnreachable},
		{"other", errors.New("weird"), ReasonUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fc.Classify(tc.err); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	wrapped := fc.Wrap(kerr.TopicAlreadyExists)
	if wrapped.Reason != ReasonTopicExists || !errors.Is(wrapped, kerr.TopicAlreadyExists) {
		t.Fatalf("wrap: %+v", wrapped)
	}
}
