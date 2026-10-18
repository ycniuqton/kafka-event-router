// Package kafkaops is the stress tooling's Kafka administration: topic/group lifecycle behind a name guard (the
// cluster is shared, only stress- resources may be created or deleted) plus offsets and consumer lag.
package kafkaops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Guard is the naming rule of every resource the stress tooling owns on shared infrastructure.
type Guard struct {
	Prefix string
}

// StressGuard is the guard used by all stress tools.
var StressGuard = Guard{Prefix: "stress-"}

// Defaults for topics the tooling creates.
var Defaults = struct {
	Replication int16
}{Replication: -1}

// Check refuses names outside the guard.
func (g Guard) Check(names ...string) error {
	for _, n := range names {
		if !strings.HasPrefix(n, g.Prefix) {
			return fmt.Errorf("refusing to touch %q: only %s* resources are managed", n, g.Prefix)
		}
	}
	return nil
}

// Admin wraps a kadm client with the guard.
type Admin struct {
	cl          *kgo.Client
	adm         *kadm.Client
	guard       Guard
	topicConfig map[string]*string
}

// Dial connects an admin client to brokers.
func Dial(brokers []string) (*Admin, error) {
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, err
	}
	return &Admin{cl: cl, adm: kadm.NewClient(cl), guard: StressGuard}, nil
}

// WithTopicConfig sets the topic-level configs used for every topic EnsureTopics creates.
func (a *Admin) WithTopicConfig(cfg map[string]string) *Admin {
	a.topicConfig = map[string]*string{}
	for k, v := range cfg {
		a.topicConfig[k] = kadm.StringPtr(v)
	}
	return a
}

// Close releases the connection.
func (a *Admin) Close() {
	a.cl.Close()
}

// EnsureTopics creates topics that do not exist yet (existing ones are kept as they are).
func (a *Admin) EnsureTopics(ctx context.Context, partitions int32, names ...string) error {
	if err := a.guard.Check(names...); err != nil || len(names) == 0 {
		return err
	}
	res, err := a.adm.CreateTopics(ctx, partitions, Defaults.Replication, a.topicConfig, names...)
	if err != nil {
		return err
	}
	for _, r := range res.Sorted() {
		if r.Err != nil && !errors.Is(r.Err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("create topic %s: %w", r.Topic, r.Err)
		}
	}
	return nil
}

// DeleteTopics deletes guarded topics; unknown topics are not an error.
func (a *Admin) DeleteTopics(ctx context.Context, names ...string) error {
	if err := a.guard.Check(names...); err != nil || len(names) == 0 {
		return err
	}
	res, err := a.adm.DeleteTopics(ctx, names...)
	if err != nil {
		return err
	}
	for _, r := range res.Sorted() {
		if r.Err != nil && !errors.Is(r.Err, kerr.UnknownTopicOrPartition) {
			return fmt.Errorf("delete topic %s: %w", r.Topic, r.Err)
		}
	}
	return nil
}

// DeleteGroups deletes guarded consumer groups; missing groups are not an error.
func (a *Admin) DeleteGroups(ctx context.Context, groups ...string) error {
	if err := a.guard.Check(groups...); err != nil || len(groups) == 0 {
		return err
	}
	res, err := a.adm.DeleteGroups(ctx, groups...)
	if err != nil {
		return err
	}
	for g, r := range res {
		if r.Err != nil && !errors.Is(r.Err, kerr.GroupIDNotFound) {
			return fmt.Errorf("delete group %s: %w", g, r.Err)
		}
	}
	return nil
}

// ListGuarded lists existing topics carrying the guard prefix (sweeping leftovers of interrupted runs).
func (a *Admin) ListGuarded(ctx context.Context, prefix string) ([]string, error) {
	if err := a.guard.Check(prefix); err != nil {
		return nil, err
	}
	td, err := a.adm.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range td.Names() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out, nil
}

// ListGroupsGuarded lists consumer groups carrying the guard prefix (sweeping leftovers of interrupted runs).
func (a *Admin) ListGroupsGuarded(ctx context.Context, prefix string) ([]string, error) {
	if err := a.guard.Check(prefix); err != nil {
		return nil, err
	}
	groups, err := a.adm.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, g := range groups.Groups() {
		if strings.HasPrefix(g, prefix) {
			out = append(out, g)
		}
	}
	return out, nil
}
