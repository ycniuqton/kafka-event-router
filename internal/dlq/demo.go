package dlq

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/kafka"
)

// DemoSeedMode selects whether routes on demo connections get seeded dead letters (ROUTER_DLQ_DEMO_SEED).
type DemoSeedMode string

const (
	DemoSeedOn  DemoSeedMode = "seed"
	DemoSeedOff DemoSeedMode = "none"
)

// DemoSeeder makes sure the DLQ of a demo route holds the demo workspace's dead letters (FR-20).
type DemoSeeder interface {
	Ensure(ctx context.Context, loc Location) error
}

// NoDemoSeeder never seeds.
type NoDemoSeeder struct{}

// Ensure implements DemoSeeder.
func (NoDemoSeeder) Ensure(context.Context, Location) error { return nil }

// DemoSeederRegistry selects the configured seeder.
type DemoSeederRegistry map[DemoSeedMode]func() DemoSeeder

// Build returns the seeder of mode (none when unknown).
func (r DemoSeederRegistry) Build(mode DemoSeedMode) DemoSeeder {
	if f, ok := r[mode]; ok {
		return f()
	}
	return NoDemoSeeder{}
}

// KafkaDemoSeeder writes DemoDeadLetterSet records into the empty DLQ topics of demo connections. The
// demo cluster lives in this process, so the topics are checked once per process (and after a restart).
type KafkaDemoSeeder struct {
	Specs   SpecLookup
	Clients ClientOpener
	Source  PartitionSource
	Routes  RouteSource
	Now     func() time.Time
	seeded  sync.Map
}

// Ensure implements DemoSeeder; non-demo targets are left alone.
func (s *KafkaDemoSeeder) Ensure(ctx context.Context, loc Location) error {
	for _, t := range loc.Targets {
		spec, err := s.Specs.Spec(ctx, t.ConnectionID)
		if err != nil || spec.ClusterKindOrDefault() != kafka.ClusterDemo {
			continue
		}
		for _, topic := range t.Topics {
			if err := s.topic(ctx, loc.Route, t.ConnectionID, topic); err != nil {
				return err
			}
		}
	}
	return nil
}

// topic seeds one DLQ topic when it is empty or missing.
func (s *KafkaDemoSeeder) topic(ctx context.Context, route RouteInfo, conn, topic string) error {
	key := conn + string(DemoKeyJoin) + topic
	if _, done := s.seeded.Load(key); done {
		return nil
	}
	set, err := s.Source.Ranges(ctx, Target{ConnectionID: conn, Topics: []string{topic}}, time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	if (DemoTopicState{}).HasRecords(set) {
		s.seeded.Store(key, true)
		return nil
	}
	if err := s.write(ctx, route, conn, topic); err != nil {
		return err
	}
	s.seeded.Store(key, true)
	return nil
}

// write produces the demo set built for the route's active version.
func (s *KafkaDemoSeeder) write(ctx context.Context, route RouteInfo, conn, topic string) error {
	target, err := s.demoTarget(ctx, route, topic)
	if err != nil {
		return err
	}
	cl, err := s.Clients.Open(ctx, conn, kafka.PurposeProducer, kgo.AllowAutoTopicCreation())
	if err != nil {
		return err
	}
	defer cl.Close()
	recs := (DemoDeadLetterSet{}).Records(target, s.Now())
	return cl.ProduceSync(ctx, (DemoRecords{}).Kafka(topic, recs)...).FirstErr()
}

// demoTarget reads the step/branch/source of the active version (or the newest version of a draft route).
func (s *KafkaDemoSeeder) demoTarget(ctx context.Context, route RouteInfo, topic string) (DemoTarget, error) {
	versions, err := s.Routes.Versions(ctx, route.ID)
	if err != nil {
		return DemoTarget{}, err
	}
	return (DemoTargetOf{}).Versions(route, topic, versions), nil
}

// DemoKeySeparator joins connection and topic in the seeded-topic index.
type DemoKeySeparator string

const DemoKeyJoin DemoKeySeparator = "/"

// DemoTopicState inspects a DLQ topic before seeding.
type DemoTopicState struct{}

// HasRecords reports whether any partition ever held a record.
func (DemoTopicState) HasRecords(set RangeSet) bool {
	for _, r := range set.Ranges {
		if r.High > 0 {
			return true
		}
	}
	return false
}

// DemoTarget is where seeded failures claim to come from.
type DemoTarget struct {
	RouteID     string
	VersionNo   int
	Branch      string
	Step        string
	SourceTopic string
}

// DemoTargetOf derives the demo target from stored versions.
type DemoTargetOf struct{}

// Versions picks the active version (else the newest) and its first JavaScript step (else first step, else filter).
func (DemoTargetOf) Versions(route RouteInfo, dlqTopic string, versions []VersionConfig) DemoTarget {
	out := DemoTarget{RouteID: route.ID, Step: string(DemoStepFallback), SourceTopic: string(kafka.DemoOrderEvents)}
	v, ok := (DemoTargetOf{}).pick(route, versions)
	if !ok {
		return out
	}
	out.VersionNo = v.No
	cfg, ok := (StepLabelsOf{}).config(v)
	if !ok || len(cfg.Branches) == 0 {
		return out
	}
	if len(cfg.Source.Topics) > 0 {
		out.SourceTopic = cfg.Source.Topics[0]
	}
	out.Branch, out.Step = cfg.Branches[0].ID, (DemoTargetOf{}).step(cfg.Branches[0])
	return out
}

func (DemoTargetOf) pick(route RouteInfo, versions []VersionConfig) (VersionConfig, bool) {
	var newest VersionConfig
	found := false
	for _, v := range versions {
		if v.ID == route.ActiveVersionID {
			return v, true
		}
		if !found || v.No > newest.No {
			newest, found = v, true
		}
	}
	return newest, found
}

func (DemoTargetOf) step(b config.Branch) string {
	if b.Transform == nil || len(b.Transform.Steps) == 0 {
		return string(DemoStepFallback)
	}
	for i, s := range b.Transform.Steps {
		if s.Kind == config.StepKindJS {
			return step.IDFor(s, i)
		}
	}
	return step.IDFor(b.Transform.Steps[0], 0)
}

// DemoStep names the step of seeded failures when the route has no transform step.
type DemoStep string

const DemoStepFallback DemoStep = "filter"

// DemoRecords converts dead letters into produce records (the DLQ writer's wire format).
type DemoRecords struct{}

// Kafka encodes recs through the shared record codec.
func (DemoRecords) Kafka(topic string, recs []record.Record) []*kgo.Record {
	out := make([]*kgo.Record, 0, len(recs))
	for _, r := range recs {
		raw := record.Codec{}.Encode(r)
		hs := make([]kgo.RecordHeader, 0, len(raw.Headers))
		for _, h := range raw.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		out = append(out, &kgo.Record{Topic: topic, Key: raw.Key, Value: raw.Value, Headers: hs, Timestamp: r.Failure.FailedAt})
	}
	return out
}

// DemoValue rewrites a demo order into the payload that made it fail.
type DemoValue interface {
	Bytes(order kafka.DemoOrderEvent) []byte
}

// DemoNullItems sends an order without items (the classic `items.reduce` TypeError).
type DemoNullItems struct{}

// Bytes implements DemoValue.
func (DemoNullItems) Bytes(o kafka.DemoOrderEvent) []byte {
	o.Items = nil
	b, _ := json.Marshal(o)
	return b
}

// DemoIntact keeps the order as produced.
type DemoIntact struct{}

// Bytes implements DemoValue.
func (DemoIntact) Bytes(o kafka.DemoOrderEvent) []byte {
	b, _ := json.Marshal(o)
	return b
}

// DemoStringAmount writes the amount as a string (output schema violation).
type DemoStringAmount struct{}

// Bytes implements DemoValue.
func (DemoStringAmount) Bytes(o kafka.DemoOrderEvent) []byte {
	b, _ := json.Marshal(map[string]any{"eventType": o.EventType, "orderId": o.OrderID, "cust_id": o.CustID,
		"email": o.Email, "amount": strconv.FormatFloat(o.Amount, 'f', 2, 64), "currency": o.Currency,
		"country": o.Country, "items": o.Items, "createdAt": o.CreatedAt})
	return b
}

// DemoTruncated cuts the JSON in half (undecodable source bytes).
type DemoTruncated struct{}

// Bytes implements DemoValue.
func (DemoTruncated) Bytes(o kafka.DemoOrderEvent) []byte {
	b := DemoIntact{}.Bytes(o)
	return b[:len(b)/2]
}

// DemoSeedingLocator seeds demo DLQ topics the first time a demo route is located; a seeding failure
// is logged and never fails the read.
type DemoSeedingLocator struct {
	Inner  Locator
	Seeder DemoSeeder
	Log    *slog.Logger
}

// Locate implements Locator.
func (l DemoSeedingLocator) Locate(ctx context.Context, routeID string) (Location, error) {
	loc, err := l.Inner.Locate(ctx, routeID)
	if err != nil {
		return loc, err
	}
	if err := l.Seeder.Ensure(ctx, loc); err != nil {
		l.Log.Warn(string(LogDemoSeedFailed), string(LogAttrRoute), routeID, string(LogAttrError), err)
	}
	return loc, nil
}
