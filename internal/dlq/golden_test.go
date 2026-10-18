package dlq

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

func goldenRecords() []record.Record {
	json := failure(1, record.ErrorStep)
	json.Ref = Ref{Topic: testDLQTopic, Partition: 0, Offset: 884162}
	json.Value = []byte(`{"orderId":"ord_123","items":null}`)
	json.Headers = []engine.Header{{Key: "traceId", Value: []byte("ab12")}, {Key: "bin", Value: []byte{0xff, 0x00}}}
	json.Failure.Stack = "TypeError: x\n    at transform (step-1.js:7:22)"
	raw := failure(2, record.ErrorDecode)
	raw.Ref = Ref{Topic: testDLQTopic, Partition: 1, Offset: 7}
	raw.Key, raw.Value = nil, []byte{0xde, 0xad, 0xbe, 0xef}
	return []record.Record{json, raw}
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("%s mismatch:\n%s", name, got)
	}
}

func TestGoldenDetailAndExport(t *testing.T) {
	route := RouteInfo{ID: "r3", Name: "Order totals"}
	recs := goldenRecords()
	views := []DetailView{}
	for _, r := range recs {
		views = append(views, Presenter{}.Detail(route, r, nil))
	}
	detail, _ := json.MarshalIndent(views[0], "", "  ")
	checkGolden(t, "detail.golden.json", detail)
	var buf bytes.Buffer
	if err := (JSONArrayExporter{}).Write(&buf, views); err != nil {
		t.Fatal(err)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, buf.Bytes(), "", "  "); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "export.golden.json", pretty.Bytes())
}
