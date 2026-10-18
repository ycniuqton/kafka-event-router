package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
)

const orderAvro = `{"type":"record","name":"Order","fields":[
 {"name":"id","type":"long"},
 {"name":"status","type":"string"},
 {"name":"customer","type":{"type":"record","name":"Customer","fields":[{"name":"email","type":"string"}]}},
 {"name":"items","type":{"type":"array","items":{"type":"record","name":"Item","fields":[{"name":"sku","type":"string"}]}}},
 {"name":"coupon","type":["null","string"],"default":null}
]}`

// registryServer serves a MemoryRegistry over the Confluent REST paths the HTTP client calls.
func registryServer(t *testing.T, mem *codectest.MemoryRegistry) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/schemas/ids/"))
		s, serr := mem.SchemaByID(r.Context(), int32(id))
		if err != nil || serr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", string(codec.RegistryContentType))
		_ = json.NewEncoder(w).Encode(s)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type p2Fixture struct {
	kafka   kafkaFixture
	svc     *preview.Service
	writer  preview.TestTopicWriter
	reg     *preview.MemoryTestTopicRegistry
	clients preview.SpecClients
	now     time.Time
}

func newP2Fixture(t *testing.T) *p2Fixture {
	t.Helper()
	fx := &p2Fixture{kafka: newKafkaFixture(t), reg: preview.NewMemoryTestTopicRegistry(), now: time.Now().UTC().Truncate(time.Second)}
	mem := codectest.NewMemoryRegistry()
	mem.Register("orders-avro-value", codec.SchemaTypeAvro, orderAvro)
	srv := registryServer(t, mem)
	spec := kafka.ConnectionSpec{BootstrapServers: fx.kafka.cluster.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext,
		SchemaRegistry: &kafka.SchemaRegistrySpec{URL: srv.URL}}
	specs := staticSpecs{spec: spec}
	fx.produceAvro(t, mem)
	clock := func() time.Time { return fx.now }
	registries, err := preview.NewRegistryProviderRegistry(specs, clock).Build(preview.SchemaRegistryControlPlane)
	if err != nil {
		t.Fatal(err)
	}
	fx.clients = preview.SpecClients{Specs: specs, Clients: kafka.DefaultClientFactoryRegistry()}
	fx.writer = preview.TestTopicWriter{Clients: fx.clients, Registry: fx.reg, Now: clock}
	fx.svc = newP2Service(t, specs, registries, fx.writer, clock)
	return fx
}

func newP2Service(t *testing.T, specs staticSpecs, registries codec.RegistryProvider, writer preview.TestTopicWriter, clock func() time.Time) *preview.Service {
	builder, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), preview.NoScripts{}, ctxhelpers.StaticSaltProvider{})
	if err != nil {
		t.Fatal(err)
	}
	builder.Registries = registries
	decoder := preview.NewSampleDecoder(registries)
	sampling := preview.KafkaSampling{Clients: preview.FranzSamplingFactory{Specs: specs, Clients: kafka.DefaultClientFactoryRegistry()}}
	return preview.NewService(preview.ServiceDeps{
		Sources:  preview.NewSampleSourceManager(preview.KafkaLatestSource{Sampling: sampling, Planner: kafka.LatestOffsetPlanner{}}, preview.UploadSource{Clock: clock}),
		Sessions: preview.NewSessionStoreManager(preview.NewMemorySessionStore(clock), clock),
		Runner:   preview.PreviewRunner{Builder: builder, Differ: preview.StructuralDiffer{}, Deadline: 10 * time.Second, Now: clock},
		Views:    preview.SampleViewBuilder{Decoder: decoder},
		Schemas: preview.SchemaBuilder{Decoder: decoder, Inferrer: preview.NewSchemaInferrer(),
			Merger: preview.RegistrySchemaMerger{Sources: preview.DefaultRegistryFieldSources(), Decoder: decoder}},
		TestTopics: writer, Metrics: preview.NewMetrics(nil), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock, SchemaJobs: 1,
	})
}

// produceAvro writes three framed Avro orders to orders-avro.
func (fx *p2Fixture) produceAvro(t *testing.T, mem *codectest.MemoryRegistry) {
	subject := "orders-avro-value"
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro, Subject: &subject}, Registry: mem})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := kadm.NewClient(fx.kafka.producer).CreateTopic(ctx, 1, 1, nil, "orders-avro"); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		v := map[string]any{"id": int64(i), "status": "PAID", "customer": map[string]any{"email": fmt.Sprintf("c%d@shop.dev", i)},
			"items": []any{map[string]any{"sku": "A-1"}}, "coupon": nil}
		raw, err := c.Encode(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		if err := fx.kafka.producer.ProduceSync(ctx, &kgo.Record{Topic: "orders-avro", Value: raw}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func (fx *p2Fixture) load(t *testing.T, ctx context.Context, format config.FormatType, caller preview.Caller) preview.LoadResponseDTO {
	t.Helper()
	res, err := fx.svc.Load(ctx, caller, preview.LoadRequest{ConnectionID: "c1", Topics: []string{"orders-avro"}, Mode: preview.ModeLatest, Count: 10, Format: format})
	if err != nil || len(res.Events) != 3 {
		t.Fatalf("load: %v %+v", err, res)
	}
	return res
}

// TestAvroSessionWithRegistry: Avro samples decode through the connection's registry, a session loaded
// as json can be switched to avro (re-decoding raw bytes), and the schema carries registry fields.
func TestAvroSessionWithRegistry(t *testing.T) {
	fx, ctx := newP2Fixture(t), integrationCtx(t)
	caller := preview.Caller{UserID: "u1"}
	res := fx.load(t, ctx, config.FormatTypeJSON, caller)
	if res.Events[0].DecodeError == nil {
		t.Fatalf("avro bytes must not decode as json: %+v", res.Events[0])
	}
	re, err := fx.svc.Reformat(ctx, caller, res.Session.ID, preview.FormatRequest{Format: config.FormatTypeAvro})
	if err != nil || re.Events[0].DecodeError != nil {
		t.Fatalf("reformat: %v %+v", err, re.Events)
	}
	value, _ := re.Events[0].Value.(map[string]any)
	if value["status"] != "PAID" || value["customer"].(map[string]any)["email"] != "c0@shop.dev" {
		t.Fatalf("decoded value = %#v", value)
	}
	fs, err := fx.svc.Schema(ctx, caller, res.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"value.customer.email", "value.items[].sku", "value.coupon"} {
		if n, ok := findSchemaNode(fs.Root, path); !ok || !n.Registry {
			t.Fatalf("schema node %s missing or not from registry: %+v", path, n)
		}
	}
	if _, err := fx.svc.Reformat(ctx, caller, res.Session.ID, preview.FormatRequest{Format: "xml"}); !errors.Is(err, preview.ErrUnknownFormat) {
		t.Fatalf("unknown format: %v", err)
	}
}

// TestMaskedSessionNeverReturnsRawValues: a session loaded by a masked caller stays masked for pages, schema and runs.
func TestMaskedSessionNeverReturnsRawValues(t *testing.T) {
	fx, ctx := newP2Fixture(t), integrationCtx(t)
	res := fx.load(t, ctx, config.FormatTypeAvro, preview.Caller{UserID: "u1", Mask: true, MaskStyle: preview.MaskTypePreserving})
	plain := preview.Caller{UserID: "u1"}
	page, err := fx.svc.Events(ctx, plain, res.Session.ID, preview.Page{})
	if err != nil {
		t.Fatal(err)
	}
	fs, _ := fx.svc.Schema(ctx, plain, res.Session.ID)
	run, err := fx.svc.Run(ctx, plain, preview.RunRequest{SessionID: res.Session.ID, Config: preview.RawConfig(avroRoute("orders-copy"))}, preview.RunPreview)
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{"load": res, "page": page, "schema": fs, "run": run} {
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), "@shop.dev") || strings.Contains(string(b), "PAID") {
			t.Fatalf("%s leaks raw values: %s", name, b)
		}
	}
	if !res.Session.Masked || !run.Masked || run.Summary.Passed != 3 {
		t.Fatalf("masked flags / run: %+v %+v", res.Session, run.Summary)
	}
}

// TestTestTopicWriteAndJanitor: a test run writes passed outputs to router.test.<route>.<ts>, never to the
// destination topic, and the janitor deletes the topic once it expired.
func TestTestTopicWriteAndJanitor(t *testing.T) {
	fx, ctx := newP2Fixture(t), integrationCtx(t)
	caller := preview.Caller{UserID: "u1"}
	res := fx.load(t, ctx, config.FormatTypeAvro, caller)
	run, err := fx.svc.Run(ctx, caller, preview.RunRequest{SessionID: res.Session.ID, Config: preview.RawConfig(jsonOutRoute()), WriteToTopic: true}, preview.RunTest)
	if err != nil || run.TestTopic == nil {
		t.Fatalf("test run: %v %+v", err, run.TestTopic)
	}
	want := "router.test.dev-live-preview-p2." + fx.now.Format("20060102150405")
	if run.TestTopic.Topic != want || run.TestTopic.Produced != 3 || !run.TestTopic.ExpiresAt.Equal(fx.now.Add(24*time.Hour)) {
		t.Fatalf("test topic = %+v", run.TestTopic)
	}
	adm := kadm.NewClient(fx.kafka.producer)
	if ends, _ := adm.ListEndOffsets(ctx, want, "orders-json"); offsetOf(ends, want) != 3 || offsetOf(ends, "orders-json") > 0 {
		t.Fatalf("produced to the wrong place: %v", ends)
	}
	janitor := preview.TestTopicJanitor{Registry: fx.reg, Deleter: preview.AdminTopicDeleter{Clients: fx.clients}, Lock: preview.LocalJanitorMutex{},
		Now: func() time.Time { return fx.now }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n, err := janitor.Tick(ctx); n != 0 || err != nil {
		t.Fatalf("nothing expired yet: %d %v", n, err)
	}
	fx.now = fx.now.Add(25 * time.Hour)
	if n, err := janitor.Tick(ctx); n != 1 || err != nil {
		t.Fatalf("janitor tick: %d %v", n, err)
	}
	if topics, _ := adm.ListTopics(ctx); topics.Has(want) {
		t.Fatalf("expired test topic still exists: %+v", topics[want])
	}
	if left, _ := fx.reg.Expired(ctx, fx.now, 10); len(left) != 0 {
		t.Fatalf("deleted topic still listed: %+v", left)
	}
}

func offsetOf(ends kadm.ListedOffsets, topic string) int64 {
	var sum int64
	ends.Each(func(o kadm.ListedOffset) {
		if o.Topic == topic && o.Err == nil {
			sum += o.Offset
		}
	})
	return sum
}

func avroRoute(topic string) string {
	return `{"apiVersion":"router/v1","name":"p2","source":{"connectionId":"c1","topics":["orders-avro"],"format":{"type":"avro"}},
"branches":[{"id":"main","destination":{"connectionId":"c1","topic":"` + topic + `","format":{"type":"json"}}}]}`
}

func jsonOutRoute() string {
	return strings.Replace(avroRoute("orders-json"), `"name":"p2"`, `"name":"Dev Live-Preview P2"`, 1)
}

func findSchemaNode(n preview.FieldNode, path string) (preview.FieldNode, bool) {
	if n.Path == path {
		return n, true
	}
	for _, c := range n.Children {
		if got, ok := findSchemaNode(c, path); ok {
			return got, true
		}
	}
	return preview.FieldNode{}, false
}

// TestSchemaTreeAddsRegistryOnlyFields: fields declared by the registry but absent from samples are added.
func TestSchemaTreeAddsRegistryOnlyFields(t *testing.T) {
	fs := preview.NewSchemaInferrer().Infer([]any{map[string]any{"value": map[string]any{"id": 1.0}}})
	tree := preview.RegistryField{Segments: []string{"value", "items", "[]", "sku"}, Type: preview.TypeString}
	merged := fs
	m := preview.SchemaTreeFor(&merged.Root)
	m.Ensure(tree)
	n, ok := findSchemaNode(merged.Root, "value.items[].sku")
	if !ok || !n.Registry || n.Frequency != 0 || n.Types[preview.TypeString] != 0 {
		t.Fatalf("registry-only node = %+v %v", n, ok)
	}
	if id, _ := findSchemaNode(merged.Root, "value.id"); id.Registry {
		t.Fatalf("sample-only node must not be flagged")
	}
}
