package preview

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// TestTopicPolicy holds the shape and lifetime of test topics (FR-22).
type TestTopicPolicy string

const (
	TestTopicPrefix     TestTopicPolicy = "router.test."
	TestTopicSeparator  TestTopicPolicy = "."
	TestTopicTimeLayout TestTopicPolicy = "20060102150405"
	TestTopicFallback   TestTopicPolicy = "draft"
	TestTopicNameFill   TestTopicPolicy = "-"
)

// TestTopicLimit holds numeric limits of test topics.
type TestTopicLimit int

const (
	TestTopicPartitions   TestTopicLimit = 1
	TestTopicReplication  TestTopicLimit = -1
	TestTopicMaxRouteName TestTopicLimit = 200
)

// TestTopicTTL is how long a test topic lives before the janitor deletes it.
const TestTopicTTL SessionPolicy = SessionPolicy(24 * time.Hour)

// testTopicUnsafe matches characters Kafka topic names do not allow.
var testTopicUnsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// TestTopicNamer builds router.test.<route>.<yyyymmddhhmmss> from the draft's route name.
type TestTopicNamer struct{}

// Name returns the topic name for route at t (UTC).
func (TestTopicNamer) Name(route string, t time.Time) string {
	slug := testTopicUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(route)), string(TestTopicNameFill))
	slug = strings.Trim(slug, string(TestTopicNameFill)+string(TestTopicSeparator))
	if len(slug) > int(TestTopicMaxRouteName) {
		slug = slug[:TestTopicMaxRouteName]
	}
	if slug == "" {
		slug = string(TestTopicFallback)
	}
	return string(TestTopicPrefix) + slug + string(TestTopicSeparator) + t.UTC().Format(string(TestTopicTimeLayout))
}

// OutputCollector gathers the encoded outputs of passed events during a test run.
type OutputCollector struct {
	mu      sync.Mutex
	records []engine.RawRecord
}

// Add keeps one encoded output.
func (c *OutputCollector) Add(r engine.RawRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r)
}

// Records returns the collected outputs.
func (c *OutputCollector) Records() []engine.RawRecord {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]engine.RawRecord(nil), c.records...)
}

// ConnectionClients opens Kafka clients of saved connections for a purpose.
type ConnectionClients interface {
	Open(ctx context.Context, connectionID string, purpose kafka.ClientPurpose) (kafka.Client, error)
}

// SpecClients resolves connection specs (control plane) and opens clients through connections' factory.
type SpecClients struct {
	Specs   ConnectionSpecSource
	Clients ClientOpener
}

// Open implements ConnectionClients.
func (c SpecClients) Open(ctx context.Context, connectionID string, purpose kafka.ClientPurpose) (kafka.Client, error) {
	spec, err := c.Specs.Spec(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	client, err := c.Clients.NewClient(ctx, spec, purpose)
	if err != nil {
		return nil, ErrTestTopicFailed.Wrap(err)
	}
	return client, nil
}

// TestTopicRequest is one test-topic write: who, which draft, and the passed outputs.
type TestTopicRequest struct {
	Caller  Caller
	RouteID string
	Refs    DraftRefs
	Records []engine.RawRecord
}

// TestTopicWriter creates the test topic on the destination connection, produces the passed outputs
// to it (never to the real destination topic) and records it for the janitor.
type TestTopicWriter struct {
	Clients  ConnectionClients
	Registry TestTopicRegistry
	Now      func() time.Time
}

// Write runs the whole test-topic step and returns its description.
func (w TestTopicWriter) Write(ctx context.Context, req TestTopicRequest) (*TestTopicDTO, error) {
	conn := req.Refs.Destination()
	if conn == "" {
		return nil, ErrNoDestination.At(string(FieldWriteToTopic))
	}
	now := w.Now().UTC()
	topic := TestTopicNamer{}.Name(req.Refs.Name, now)
	client, err := w.Clients.Open(ctx, conn, kafka.PurposeProducer)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := (TestTopicCreator{}).Create(ctx, client.Admin(), topic); err != nil {
		return nil, err
	}
	if err := (TestTopicProducer{}).Produce(ctx, client.Raw(), topic, req.Records); err != nil {
		return nil, err
	}
	rec := TestTopic{RouteID: req.RouteID, ConnectionID: conn, Topic: topic, Produced: len(req.Records), CreatedBy: req.Caller.UserID,
		CreatedAt: now, ExpiresAt: now.Add(TestTopicTTL.Duration())}
	if _, err := w.Registry.Record(ctx, rec); err != nil {
		return nil, err
	}
	return &TestTopicDTO{ConnectionID: conn, Topic: topic, Produced: rec.Produced, ExpiresAt: rec.ExpiresAt}, nil
}

// TestTopicCreator creates the single-partition test topic with a 24 h retention.
type TestTopicCreator struct{}

// Create maps authorization failures to 403 preview/test-topic-denied.
func (TestTopicCreator) Create(ctx context.Context, admin kafka.Admin, topic string) error {
	_, err := admin.CreateTopic(ctx, kafka.TopicSpec{
		Name: topic, Partitions: int32(TestTopicPartitions), ReplicationFactor: int16(TestTopicReplication),
		RetentionMs: TestTopicTTL.Duration().Milliseconds(),
	})
	switch {
	case err == nil, errors.Is(err, kerr.TopicAlreadyExists):
		return nil
	case KafkaAuthErrors{}.Denied(err):
		return ErrTestTopicDenied.At(string(FieldWriteToTopic)).Wrap(err)
	}
	return ErrTestTopicFailed.Wrap(err)
}

// KafkaAuthErrors classifies broker authorization failures.
type KafkaAuthErrors struct{}

// Denied reports a topic or cluster authorization failure.
func (KafkaAuthErrors) Denied(err error) bool {
	return errors.Is(err, kerr.TopicAuthorizationFailed) || errors.Is(err, kerr.ClusterAuthorizationFailed)
}

// TestTopicProducer writes outputs to the test topic, keeping keys, values and headers.
type TestTopicProducer struct{}

// Produce sends every record synchronously (acks=all from the producer profile).
func (TestTopicProducer) Produce(ctx context.Context, cl *kgo.Client, topic string, recs []engine.RawRecord) error {
	if len(recs) == 0 {
		return nil
	}
	out := make([]*kgo.Record, 0, len(recs))
	for _, r := range recs {
		out = append(out, &kgo.Record{Topic: topic, Key: r.Key, Value: r.Value, Headers: TestTopicProducer{}.headers(r.Headers)})
	}
	if err := cl.ProduceSync(ctx, out...).FirstErr(); err != nil {
		if (KafkaAuthErrors{}).Denied(err) {
			return ErrTestTopicDenied.At(string(FieldWriteToTopic)).Wrap(err)
		}
		return ErrTestTopicFailed.Wrap(err)
	}
	return nil
}

func (TestTopicProducer) headers(hs []engine.Header) []kgo.RecordHeader {
	out := make([]kgo.RecordHeader, 0, len(hs))
	for _, h := range hs {
		out = append(out, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}
	return out
}
