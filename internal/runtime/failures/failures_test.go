package failures

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/kafka"
)

func TestDispositionTable(t *testing.T) {
	cases := map[domain.ErrorCategory]domain.Disposition{
		domain.CategoryDecode:               domain.DispositionApplyPolicy,
		domain.CategoryStep:                 domain.DispositionApplyPolicy,
		domain.CategoryStepTimeout:          domain.DispositionRetryInMemory,
		domain.CategoryOutputInvalid:        domain.DispositionApplyPolicy,
		domain.CategoryDestinationTransient: domain.DispositionBackoffRetry,
		domain.CategorySourceTransient:      domain.DispositionBackoffRetry,
		domain.CategoryDestinationPermanent: domain.DispositionHaltRoute,
		domain.CategorySourceFatal:          domain.DispositionHaltRoute,
		domain.ErrorCategory("unknown"):     domain.DispositionHaltRoute,
	}
	for cat, want := range cases {
		if got := (DispositionTable{}).For(cat); got != want {
			t.Errorf("%s: got %s want %s", cat, got, want)
		}
	}
}

func TestClassifier(t *testing.T) {
	cases := []struct {
		err  error
		want domain.ErrorCategory
	}{
		{&engine.ProcessError{Type: engine.ErrDecode}, domain.CategoryDecode},
		{&engine.ProcessError{Type: engine.ErrStepTimeout}, domain.CategoryStepTimeout},
		{&engine.ProcessError{Type: engine.ErrStep}, domain.CategoryStep},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid}, domain.CategoryOutputInvalid},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: errors.New("missing field total")}, domain.CategoryOutputInvalid},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: context.DeadlineExceeded}, domain.CategoryDestinationTransient},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: registryOutage{}}, domain.CategoryDestinationTransient},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: fmt.Errorf("subject orders-value: %w", codec.ErrTransient)}, domain.CategoryDestinationTransient},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: codec.ErrSchemaNotFound}, domain.CategoryOutputInvalid},
		{&engine.ProcessError{Type: engine.ErrOutputInvalid, Cause: &net.OpError{Op: "dial", Err: errors.New("refused")}}, domain.CategoryDestinationTransient},
		{&engine.ProcessError{Type: engine.ErrLimit}, domain.CategoryStep},
		{kerr.NotLeaderForPartition, domain.CategoryDestinationTransient},
		{kerr.TopicAuthorizationFailed, domain.CategoryDestinationPermanent},
		{kerr.MessageTooLarge, domain.CategoryDestinationPermanent},
		{kerr.UnknownTopicOrPartition, domain.CategoryDestinationPermanent},
		{kgo.ErrRecordTimeout, domain.CategoryDestinationTransient},
		{context.Canceled, domain.CategoryDestinationTransient},
		{errors.New("mystery"), domain.CategoryDestinationPermanent},
	}
	for _, c := range cases {
		if got := (DefaultClassifier{}).Classify(c.err); got != c.want {
			t.Errorf("%v: got %s want %s", c.err, got, c.want)
		}
	}
	if (DefaultClassifier{}).ClassifySource(kerr.GroupAuthorizationFailed) != domain.CategorySourceFatal {
		t.Error("group authorization should be fatal")
	}
	if (DefaultClassifier{}).ClassifySource(kerr.UnknownTopicOrPartition) != domain.CategorySourceTransient {
		t.Error("unknown topic should be transient")
	}
}

// registryOutage is a codec error that knows it may heal (schema registry 5xx).
type registryOutage struct{}

func (registryOutage) Error() string   { return "schema registry unavailable" }
func (registryOutage) Temporary() bool { return true }

type nopWriter struct{ err error }

func (w nopWriter) Write(_ context.Context, _ Failure, onAck delivery.AckFunc) { onAck(w.err) }

func TestErrorPolicyManager(t *testing.T) {
	m := NewDefaultPolicyManager(nopWriter{})
	cases := []struct {
		policy  *config.ErrorPolicy
		outcome domain.RecordOutcome
		halt    bool
	}{
		{nil, domain.OutcomeDeadLettered, false},
		{&config.ErrorPolicy{OnError: config.ErrorActionDLQ}, domain.OutcomeDeadLettered, false},
		{&config.ErrorPolicy{OnError: config.ErrorActionSkip}, domain.OutcomeSkipped, false},
		{&config.ErrorPolicy{OnError: config.ErrorActionPause}, "", true},
	}
	for _, c := range cases {
		var gotOutcome domain.RecordOutcome
		var gotErr error
		m.Resolve(c.policy).Handle(context.Background(), Failure{Category: domain.CategoryDecode}, func(o domain.RecordOutcome, err error) { gotOutcome, gotErr = o, err })
		var h *HaltError
		if gotOutcome != c.outcome || errors.As(gotErr, &h) != c.halt {
			t.Errorf("policy %+v: outcome %s err %v", c.policy, gotOutcome, gotErr)
		}
	}
	retries := 5
	if m.MaxRetries(&config.ErrorPolicy{MaxRetries: &retries}) != 5 || m.MaxRetries(nil) != int(DefaultMaxRetries) {
		t.Error("max retries")
	}
	if !m.UsesDLQ(nil) || m.UsesDLQ(&config.ErrorPolicy{OnError: config.ErrorActionSkip}) {
		t.Error("uses dlq")
	}
}

func TestDLQTargetResolver(t *testing.T) {
	dlqConn, dlqTopic := "dlq-conn", "custom.dlq"
	base := func() *config.RouteConfig {
		return &config.RouteConfig{Name: "Orders Paid", Source: config.Source{ConnectionID: "src"},
			Branches: []config.Branch{{ID: "main", Destination: config.Destination{ConnectionID: "dst"}}}}
	}
	cfg := base()
	if got := (DLQTargetResolver{Default: domain.DLQTargetSource}).Resolve(cfg); got.ConnectionID != "src" || got.Topic != "router.dlq.orders-paid" {
		t.Fatalf("default: %+v", got)
	}
	if got := (DLQTargetResolver{Default: domain.DLQTargetDestination}).Resolve(cfg); got.ConnectionID != "dst" {
		t.Fatalf("destination default: %+v", got)
	}
	cfg.ErrorPolicy = &config.ErrorPolicy{DLQConnectionID: &dlqConn, DLQTopic: &dlqTopic}
	if got := (DLQTargetResolver{Default: domain.DLQTargetDestination}).Resolve(cfg); got.ConnectionID != dlqConn || got.Topic != dlqTopic {
		t.Fatalf("explicit: %+v", got)
	}
}

type fakeAdmin struct {
	kafka.Admin
	createErr error
}

func (fakeAdmin) DescribeTopics(context.Context, ...string) ([]kafka.TopicInfo, error) {
	return nil, kerr.UnknownTopicOrPartition
}

func (a fakeAdmin) CreateTopic(context.Context, kafka.TopicSpec) (kafka.TopicInfo, error) {
	return kafka.TopicInfo{}, a.createErr
}

func TestTopicEnsurerHaltsOnlyOnPermanentErrors(t *testing.T) {
	var h *HaltError
	if err := (TopicEnsurer{}).Ensure(context.Background(), fakeAdmin{createErr: kerr.TopicAuthorizationFailed}, "t", 1); !errors.As(err, &h) || h.Reason != domain.HaltDLQTopicUnavailable {
		t.Fatalf("ACL failure: %v", err)
	}
	if err := (TopicEnsurer{}).Ensure(context.Background(), fakeAdmin{createErr: kerr.RequestTimedOut}, "t", 1); err == nil || errors.As(err, &h) {
		t.Fatalf("timeout must be retried, got %v", err)
	}
	if err := (TopicEnsurer{}).Ensure(context.Background(), fakeAdmin{createErr: kerr.TopicAlreadyExists}, "t", 1); err != nil {
		t.Fatalf("existing topic: %v", err)
	}
}
