package codec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// registerSubject registers schema text under subject through the Confluent REST API.
func registerSubject(t *testing.T, baseURL, subject string, typ codec.SchemaType, text string) int32 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"schema": text, "schemaType": typ})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/subjects/%s/versions", baseURL, subject), bytes.NewReader(body))
	req.Header.Set("Content-Type", string(codec.RegistryContentType))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		ID int32 `json:"id"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil {
		t.Fatalf("register %s: status %d", subject, resp.StatusCode)
	}
	return out.ID
}

// TestAvroAgainstRealRegistry runs the Avro codec against a real Confluent-compatible
// registry (ROUTER_TEST_SCHEMA_REGISTRY_URL or the testkit container provider).
func TestAvroAgainstRealRegistry(t *testing.T) {
	ep := testkit.NewManager().Require(t, testkit.ServiceSchemaRegistry)
	subject := fmt.Sprintf("dev-pipeline-engine-it-%d-value", time.Now().UnixNano())
	id := registerSubject(t, ep.URL, subject, codec.SchemaTypeAvro, orderSchema)
	reg := codec.NewCachedRegistry(codec.NewHTTPRegistry(codec.RegistryConfig{URL: ep.URL}, nil), codec.DefaultSchemaCacheSize)
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro, Subject: &subject}, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	raw, err := c.Encode(ctx, orderValue())
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := codec.Framing{}.Parse(raw)
	if frame.SchemaID != id {
		t.Fatalf("encoded with id %d, registry returned %d", frame.SchemaID, id)
	}
	got, err := c.Decode(ctx, raw)
	if err != nil || !reflect.DeepEqual(got, orderValue()) {
		t.Fatalf("round trip: %v\n%#v", err, got)
	}
	if _, err := c.Decode(ctx, codec.Framing{}.Write(id+100000, nil)); err == nil {
		t.Fatal("unknown schema id must fail")
	}
}
