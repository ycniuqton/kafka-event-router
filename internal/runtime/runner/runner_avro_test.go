// Avro destination (RR-103, RR-106): outputs that do not fit the destination schema are dead-lettered as
// output_invalid with their branch; a schema-registry outage at encode time is retried, never dead-lettered.
package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
)

const avroRoute = `{"apiVersion":"router/v1","name":"orders-avro","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"dlq"},
"branches":[{"id":"main","filter":null,"destination":{"connectionId":"c1","topic":"out","format":{"type":"avro","subject":"orders-value"}}}]}`

const orderSchema = `{"type":"record","name":"Order","fields":[{"name":"id","type":"long"},{"name":"total","type":"double"}]}`

// orderSchemaID is the registry id of orderSchema.
const orderSchemaID int32 = 7

// memRegistry is an in-memory schema registry whose Latest fails transiently while down.
type memRegistry struct {
	mu   sync.Mutex
	down bool
}

func (r *memRegistry) setDown(down bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.down = down
}

func (r *memRegistry) schema() codec.RegisteredSchema {
	return codec.RegisteredSchema{ID: orderSchemaID, Subject: "orders-value", Version: 1, Text: orderSchema}
}

func (r *memRegistry) SchemaByID(_ context.Context, id int32) (codec.RegisteredSchema, error) {
	if id != orderSchemaID {
		return codec.RegisteredSchema{}, codec.ErrSchemaNotFound
	}
	return r.schema(), nil
}

func (r *memRegistry) Latest(_ context.Context, subject string) (codec.RegisteredSchema, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.down {
		return codec.RegisteredSchema{}, fmt.Errorf("registry 503: %w", codec.ErrTransient)
	}
	return r.schema(), nil
}

func (r *memRegistry) Version(ctx context.Context, subject string, _ int) (codec.RegisteredSchema, error) {
	return r.Latest(ctx, subject)
}

// registryAvro binds the Avro factory to one registry (the per-connection provider of pipeline-engine).
type registryAvro struct{ reg codec.SchemaRegistry }

func (registryAvro) Format() config.FormatType { return config.FormatTypeAvro }

func (f registryAvro) New(spec codec.CodecSpec) (codec.Codec, error) {
	return codec.AvroFactory{}.New(codec.CodecSpec{Format: spec.Format, Registry: f.reg})
}

func avroVersion(t *testing.T, c *kfake.Cluster, reg codec.SchemaRegistry) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(avroRoute))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		t.Fatal(err)
	}
	regs := compile.DefaultRegistries()
	regs.Codecs = codec.NewFactoryManager(codec.JSONFactory{}, codec.StringFactory{}, codec.BytesFactory{}, registryAvro{reg: reg})
	p, err := compile.NewPipelineCompiler(regs).Compile(context.Background(), doc, engine.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := kafka.ConnectionSpec{ID: "c1", BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "ra", VersionID: "v1", VersionNo: 3}, RouteName: "orders-avro", Config: cfg,
		Pipeline: p, Connections: map[string]kafka.ConnectionSpec{"c1": spec}}
}

func orders(valid, invalid int) []string {
	out := []string{}
	for i := 0; i < valid; i++ {
		b, _ := json.Marshal(map[string]any{"id": i, "total": float64(i) + 0.5})
		out = append(out, string(b))
	}
	for i := 0; i < invalid; i++ {
		out = append(out, fmt.Sprintf(`{"id":"not-a-long-%d"}`, i))
	}
	return out
}

func readDeadLetters(t *testing.T, c *kfake.Cluster, topic string, want int) []record.Record {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out := []record.Record{}
	for len(out) < want && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			rec, err := record.Codec{}.Decode(delivery.RecordMapper{}.FromKafka(r))
			if err == nil {
				out = append(out, rec)
			}
		})
	}
	return out
}

func TestAvroRouteDeadLettersOutputsThatViolateTheSchema(t *testing.T) {
	c := cluster(t)
	valid, invalid := 20, 4
	produce(t, c, orders(valid, invalid)...)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), avroVersion(t, c, &memRegistry{})); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "valid outputs", func() bool { return countRecords(t, c, "out") == valid })
	eventually(t, "every record committed", func() bool { return committed(t, c, "router.ra") == int64(valid+invalid) })
	letters := readDeadLetters(t, c, "router.dlq.orders-avro", invalid)
	if len(letters) != invalid {
		t.Fatalf("dead letters = %d, want %d", len(letters), invalid)
	}
	for _, l := range letters {
		if l.Failure.ErrorType != record.ErrorOutputInvalid || l.Failure.Branch != "main" || l.Failure.VersionNo != 3 || l.Source.Topic != "in" {
			t.Fatalf("unexpected dead letter: %+v", l.Failure)
		}
	}
	assertAvroOutput(t, c)
}

// assertAvroOutput decodes one produced record with the destination schema.
func assertAvroOutput(t *testing.T, c *kfake.Cluster) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics("out"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var raw []byte
	for raw == nil && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) { raw = r.Value })
	}
	dec, _ := codec.AvroFactory{}.New(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro}, Registry: &memRegistry{}})
	v, err := dec.Decode(context.Background(), raw)
	if err != nil {
		t.Fatalf("output is not Avro of the destination schema: %v", err)
	}
	if fields, ok := v.(map[string]any); !ok || fields["total"] == nil {
		t.Fatalf("decoded output %#v", v)
	}
}

func TestRegistryOutageAtEncodeIsRetriedNotDeadLettered(t *testing.T) {
	c := cluster(t)
	reg := &memRegistry{down: true}
	valid := 10
	produce(t, c, orders(valid, 0)...)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), avroVersion(t, c, reg)); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	time.Sleep(time.Second)
	if n := countRecords(t, c, "out"); n > 0 {
		t.Fatalf("produced %d records while the registry was down", n)
	}
	if got := committed(t, c, "router.ra"); got > 0 {
		t.Fatalf("committed %d records past a registry outage", got)
	}
	reg.setDown(false)
	eventually(t, "outputs after the registry recovered", func() bool { return countRecords(t, c, "out") == valid })
	eventually(t, "every record committed", func() bool { return committed(t, c, "router.ra") == int64(valid) })
	if n := countRecords(t, c, "router.dlq.orders-avro"); n > 0 {
		t.Fatalf("registry outage dead-lettered %d good records", n)
	}
}
