package overview

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/query"
)

func TestLoadReturnsTuningValues(t *testing.T) {
	q := query.NewMockQuerier(func() time.Time { return time.Unix(1700000000, 0) })
	m, err := newService(q, fakeLookup{}).Load(context.Background(), "r1", metrics.Range15m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Tuning.InFlightLimit == nil || m.Tuning.JSInstances == nil || m.Tuning.JSQueueWaitP95 == nil {
		t.Fatalf("tuning %+v", m.Tuning)
	}
	body, _ := json.Marshal(m)
	if !strings.Contains(string(body), `"tuning":{"inFlightLimit":`) {
		t.Fatalf("tuning must be in the response: %s", body)
	}
}

func TestVectorValueIsNullWithoutSeries(t *testing.T) {
	vv := VectorValue{}
	if vv.Of(nil) != nil || vv.Of(query.Vector{{Value: math.NaN()}}) != nil {
		t.Fatal("empty or NaN must be null")
	}
	if v := vv.Of(query.Vector{{Value: 4200}}); v == nil || *v != 4200 {
		t.Fatalf("value %v", v)
	}
}
