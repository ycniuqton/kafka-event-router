package connections

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/kafka"
)

// TopicMessage holds the user-facing texts of create-topic validation.
type TopicMessage string

const (
	MsgTopicNameRequired TopicMessage = "Enter a topic name."
	MsgTopicNameInvalid  TopicMessage = "Use up to 249 letters, digits, dots, dashes or underscores (not \".\" or \"..\")."
	MsgPartitionsRange   TopicMessage = "Partitions must be between 1 and 1000."
	MsgReplicationRange  TopicMessage = "Replication factor must be between 1 and 32, or -1 for the broker default."
	MsgRetentionRange    TopicMessage = "Retention must be positive, or -1 to keep forever."
)

// TopicNamePattern is Kafka's legal topic name shape.
var TopicNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,249}$`)

// TopicRequest is a create-topic request; nil fields take TopicDefaults.
type TopicRequest struct {
	Name              string
	Partitions        *int32
	ReplicationFactor *int16
	RetentionMs       *int64
	Configs           map[string]string
}

// TopicRequestValidator validates and defaults a create-topic request.
type TopicRequestValidator struct{}

// Spec returns the defaulted spec or a 422 validation error.
func (TopicRequestValidator) Spec(r TopicRequest) (kafka.TopicSpec, error) {
	spec := kafka.TopicSpec{Name: r.Name, Partitions: int32(DefaultTopicPartitions), ReplicationFactor: int16(DefaultReplication), RetentionMs: int64(DefaultRetentionMs), Configs: r.Configs}
	if r.Partitions != nil {
		spec.Partitions = *r.Partitions
	}
	if r.ReplicationFactor != nil {
		spec.ReplicationFactor = *r.ReplicationFactor
	}
	if r.RetentionMs != nil {
		spec.RetentionMs = *r.RetentionMs
	}
	verr := api.NewValidationError()
	if r.Name == "" {
		verr.Add(string(PathTopicName), string(kafka.CodeRequired), string(MsgTopicNameRequired))
	} else if !TopicNamePattern.MatchString(r.Name) || r.Name == "." || r.Name == ".." {
		verr.Add(string(PathTopicName), string(kafka.CodeInvalid), string(MsgTopicNameInvalid))
	}
	if spec.Partitions < 1 || spec.Partitions > int32(MaxTopicPartitions) {
		verr.Add(string(PathTopicPartitions), string(kafka.CodeInvalid), string(MsgPartitionsRange))
	}
	if spec.ReplicationFactor == 0 || spec.ReplicationFactor < int16(DefaultReplication) || spec.ReplicationFactor > int16(MaxReplicationFactor) {
		verr.Add(string(PathTopicReplication), string(kafka.CodeInvalid), string(MsgReplicationRange))
	}
	if spec.RetentionMs == 0 || spec.RetentionMs < -1 {
		verr.Add(string(PathTopicRetention), string(kafka.CodeInvalid), string(MsgRetentionRange))
	}
	return spec, verr.OrNil()
}

// CreatedTopic is the create-topic response.
type CreatedTopic struct {
	Name              string `json:"name"`
	Partitions        int32  `json:"partitions"`
	ReplicationFactor int16  `json:"replicationFactor"`
	RetentionMs       int64  `json:"retentionMs"`
}

// TopicCreator creates topics on connections that opted in (allowTopicCreate).
type TopicCreator struct {
	resolver  *SpecResolver
	clients   ClientOpener
	failures  ClusterFailures
	validator TopicRequestValidator
	auditor   Auditor
}

// NewTopicCreator builds the creator.
func NewTopicCreator(resolver *SpecResolver, clients ClientOpener, failures ClusterFailures) *TopicCreator {
	return &TopicCreator{resolver: resolver, clients: clients, failures: failures}
}

// Create validates the request, checks the opt-in and creates the topic.
func (t *TopicCreator) Create(ctx context.Context, id uuid.UUID, r TopicRequest) (CreatedTopic, error) {
	c, spec, err := t.resolver.Resolve(ctx, id)
	if err != nil {
		return CreatedTopic{}, err
	}
	if !c.AllowTopicCreate {
		return CreatedTopic{}, &TopicCreateDisabledError{}
	}
	topic, err := t.validator.Spec(r)
	if err != nil {
		return CreatedTopic{}, err
	}
	created, err := t.createOnCluster(ctx, spec, topic)
	if err != nil {
		return CreatedTopic{}, err
	}
	audited := TopicAudit{Topic: created.Name, Partitions: created.Partitions, RetentionMs: created.RetentionMs}
	return created, t.auditor.Record(ctx, ActionTopicCreated, c, nil, audited)
}

func (t *TopicCreator) createOnCluster(ctx context.Context, spec kafka.ConnectionSpec, topic kafka.TopicSpec) (CreatedTopic, error) {
	client, err := t.clients.NewClient(ctx, spec, kafka.PurposeAdmin)
	if err != nil {
		return CreatedTopic{}, t.failures.Wrap(StepAuthenticated, err)
	}
	defer client.Close()
	info, err := client.Admin().CreateTopic(ctx, topic)
	if errors.Is(err, kerr.TopicAlreadyExists) {
		return CreatedTopic{}, &TopicExistsError{}
	}
	if err != nil {
		return CreatedTopic{}, t.failures.Wrap(StepTopicAdmin, err)
	}
	out := CreatedTopic{Name: info.Name, Partitions: int32(info.Partitions), ReplicationFactor: int16(info.ReplicationFactor), RetentionMs: topic.RetentionMs}
	if out.Partitions <= 0 {
		out.Partitions = topic.Partitions
	}
	return out, nil
}
